package docker

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/docker/go-connections/nat"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/internal/serialization"
	"github.com/yusing/godoxy/internal/types"
	gperr "github.com/yusing/goutils/errs"
)

var DummyContainer = new(types.Container)

var EnvDockerHost = os.Getenv("DOCKER_HOST")

var (
	ErrNetworkNotFound = errors.New("network not found")
	ErrNoNetwork       = errors.New("no network found")
)

func FromDocker(c *container.Summary, dockerHost string) (res *types.Container) {
	actualLabels := maps.Clone(c.Labels)

	_, isExplicit := c.Labels[LabelAliases]
	helper := containerHelper{c}
	if !isExplicit {
		// walk through all labels to check if any label starts with NSProxy.
		for lbl := range c.Labels {
			if strings.HasPrefix(lbl, NSProxy+".") {
				isExplicit = true
				break
			}
		}
	}
	network := helper.getDeleteLabel(LabelNetwork)

	isExcluded, _ := strconv.ParseBool(helper.getDeleteLabel(LabelExclude))
	res = &types.Container{
		DockerHost:    dockerHost,
		Image:         helper.parseImage(),
		ContainerName: helper.getName(),
		ContainerID:   c.ID,

		Labels:       c.Labels,
		ActualLabels: actualLabels,

		Mounts: helper.getMounts(),

		Network:            network,
		PublicPortMapping:  helper.getPublicPortMapping(),
		PrivatePortMapping: helper.getPrivatePortMapping(),

		Aliases:           helper.getAliases(),
		IsExcluded:        isExcluded,
		IsExplicit:        isExplicit,
		IsHostNetworkMode: c.HostConfig.NetworkMode == "host",
		Running:           c.Status == "running" || c.State == "running",
		State:             c.State,
	}

	if agent.IsDockerHostAgent(dockerHost) {
		var ok bool
		res.Agent, ok = agent.GetAgent(dockerHost)
		if !ok {
			addError(res, fmt.Errorf("agent %q not found", dockerHost))
		}
	}

	setPrivateHostname(res, helper)
	setPublicHostname(res)
	loadDeleteIdlewatcherLabels(res, helper)

	if res.PrivateHostname == "" && res.PublicHostname == "" && res.Running {
		addError(res, ErrNoNetwork)
	}
	return res
}

func IsBlacklisted(c *types.Container) bool {
	return IsBlacklistedImage(c.Image) || isDatabase(c)
}

func UpdatePorts(c *types.Container) error {
	dockerClient, err := NewClient(c.DockerHost)
	if err != nil {
		return err
	}
	defer dockerClient.Close()

	inspect, err := dockerClient.ContainerInspect(context.Background(), c.ContainerID, client.ContainerInspectOptions{})
	if err != nil {
		return err
	}

	for port := range inspect.Container.Config.ExposedPorts {
		proto, portStr := nat.SplitProtoPort(port.String())
		portInt, _ := nat.ParsePort(portStr)
		if portInt == 0 {
			continue
		}
		c.PublicPortMapping[portInt] = container.PortSummary{
			PublicPort:  uint16(portInt), //nolint:gosec
			PrivatePort: uint16(portInt), //nolint:gosec
			Type:        proto,
		}
	}
	return nil
}

func DockerComposeProject(c *types.Container) string {
	return c.Labels["com.docker.compose.project"]
}

func DockerComposeService(c *types.Container) string {
	return c.Labels["com.docker.compose.service"]
}

func Dependencies(c *types.Container) []string {
	deps := c.Labels[LabelDependsOn]
	if deps == "" {
		deps = c.Labels["com.docker.compose.depends_on"]
	}
	return strings.Split(deps, ",")
}

var databaseMPs = map[string]struct{}{
	"/var/lib/postgresql/data": {},
	"/var/lib/mysql":           {},
	"/var/lib/mongodb":         {},
	"/var/lib/mariadb":         {},
	"/var/lib/memcached":       {},
	"/var/lib/rabbitmq":        {},
}

func isDatabase(c *types.Container) bool {
	if c.Mounts != nil { // only happens in test
		for _, m := range c.Mounts.Iter {
			if _, ok := databaseMPs[m]; ok {
				return true
			}
		}
	}

	for _, v := range c.PrivatePortMapping {
		switch v.PrivatePort {
		// postgres, mysql or mariadb, redis, memcached, mongodb
		case 5432, 3306, 6379, 11211, 27017:
			return true
		}
	}
	return false
}

func isLocal(c *types.Container) bool {
	if strings.HasPrefix(c.DockerHost, "unix://") {
		return true
	}
	// treat it as local if the docker host is the same as the environment variable
	if c.DockerHost == EnvDockerHost {
		return true
	}
	url, err := url.Parse(c.DockerHost)
	if err != nil {
		return false
	}
	hostname := url.Hostname()
	ip := net.ParseIP(hostname)
	if ip != nil {
		return ip.IsLoopback() || ip.IsUnspecified()
	}
	return hostname == "localhost"
}

func setPublicHostname(c *types.Container) {
	if !c.Running {
		return
	}
	if isLocal(c) {
		c.PublicHostname = "127.0.0.1"
		return
	}
	url, err := url.Parse(c.DockerHost)
	if err != nil {
		c.PublicHostname = "127.0.0.1"
		return
	}
	c.PublicHostname = url.Hostname()
}

func setPrivateHostname(c *types.Container, helper containerHelper) {
	if !isLocal(c) && c.Agent == nil {
		return
	}
	if helper.NetworkSettings == nil {
		return
	}
	if c.Network != "" {
		v, ok := helper.NetworkSettings.Networks[c.Network]
		if ok && v.IPAddress.IsValid() {
			c.PrivateHostname = v.IPAddress.String()
			return
		}
		// try {project_name}_{network_name}
		if proj := DockerComposeProject(c); proj != "" {
			oldNetwork, newNetwork := c.Network, fmt.Sprintf("%s_%s", proj, c.Network)
			if newNetwork != oldNetwork {
				v, ok = helper.NetworkSettings.Networks[newNetwork]
				if ok && v.IPAddress.IsValid() {
					c.Network = newNetwork // update network to the new one
					c.PrivateHostname = v.IPAddress.String()
					return
				}
			}
		}
		nearest := gperr.DoYouMeanField(c.Network, helper.NetworkSettings.Networks)
		addError(c, fmt.Errorf("network %q not found, %w", c.Network, nearest))
		return
	}
	// fallback to first network if no network is specified
	for k, v := range helper.NetworkSettings.Networks {
		if v.IPAddress.IsValid() {
			c.Network = k // update network to the first network
			c.PrivateHostname = v.IPAddress.String()
			return
		}
	}
}

func loadDeleteIdlewatcherLabels(c *types.Container, helper containerHelper) {
	hasIdleTimeout := false
	cfg := make(map[string]any, len(idlewatcherLabels))
	for lbl, key := range idlewatcherLabels {
		value := helper.getDeleteLabel(lbl)
		if value == "" {
			continue
		}
		cfg[key] = value
		switch lbl {
		case LabelIdleTimeout:
			hasIdleTimeout = true
		case LabelDependsOn:
			cfg[key] = Dependencies(c)
		}
	}

	// set only if idlewatcher is enabled
	if hasIdleTimeout {
		idwCfg := new(types.IdlewatcherConfig)
		idwCfg.Docker = &types.DockerConfig{
			DockerHost:    c.DockerHost,
			ContainerID:   c.ContainerID,
			ContainerName: c.ContainerName,
		}

		err := serialization.MapUnmarshalValidate(cfg, idwCfg)
		if err != nil {
			addError(c, err)
		} else {
			c.IdlewatcherConfig = idwCfg
		}
	}
}

func addError(c *types.Container, err error) {
	if c.Errors == nil {
		c.Errors = new(types.ContainerError)
	}
	c.Errors.Add(err)
}
