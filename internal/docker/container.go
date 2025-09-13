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

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/serialization"
	"github.com/yusing/go-proxy/internal/types"
	"github.com/yusing/go-proxy/internal/utils"
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
	return
}

func IsBlacklisted(c *types.Container) bool {
	return IsBlacklistedImage(c.Image) || isDatabase(c)
}

func UpdatePorts(c *types.Container) error {
	client, err := NewClient(c.DockerHost)
	if err != nil {
		return err
	}
	defer client.Close()

	inspect, err := client.ContainerInspect(context.Background(), c.ContainerID)
	if err != nil {
		return err
	}

	for port := range inspect.Config.ExposedPorts {
		proto, portStr := nat.SplitProtoPort(string(port))
		portInt, _ := nat.ParsePort(portStr)
		if portInt == 0 {
			continue
		}
		c.PublicPortMapping[portInt] = container.Port{
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
		if ok {
			c.PrivateHostname = v.IPAddress
			return
		}
		// try {project_name}_{network_name}
		if proj := DockerComposeProject(c); proj != "" {
			oldNetwork, newNetwork := c.Network, fmt.Sprintf("%s_%s", proj, c.Network)
			if newNetwork != oldNetwork {
				v, ok = helper.NetworkSettings.Networks[newNetwork]
				if ok {
					c.Network = newNetwork // update network to the new one
					c.PrivateHostname = v.IPAddress
					return
				}
			}
		}
		nearest := gperr.DoYouMean(utils.NearestField(c.Network, helper.NetworkSettings.Networks))
		addError(c, fmt.Errorf("network %q not found, %w", c.Network, nearest))
		return
	}
	// fallback to first network if no network is specified
	for k, v := range helper.NetworkSettings.Networks {
		if v.IPAddress != "" {
			c.Network = k // update network to the first network
			c.PrivateHostname = v.IPAddress
			return
		}
	}
}

func loadDeleteIdlewatcherLabels(c *types.Container, helper containerHelper) {
	cfg := map[string]any{
		"idle_timeout":   helper.getDeleteLabel(LabelIdleTimeout),
		"wake_timeout":   helper.getDeleteLabel(LabelWakeTimeout),
		"stop_method":    helper.getDeleteLabel(LabelStopMethod),
		"stop_timeout":   helper.getDeleteLabel(LabelStopTimeout),
		"stop_signal":    helper.getDeleteLabel(LabelStopSignal),
		"start_endpoint": helper.getDeleteLabel(LabelStartEndpoint),
		"depends_on":     Dependencies(c),
	}

	// ensure it's deleted from labels
	helper.getDeleteLabel(LabelDependsOn)

	// set only if idlewatcher is enabled
	idleTimeout := cfg["idle_timeout"]
	if idleTimeout != "" {
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
