package docker

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/internal/gperr"
	idlewatcher "github.com/yusing/go-proxy/internal/idlewatcher/types"
	"github.com/yusing/go-proxy/internal/serialization"
	"github.com/yusing/go-proxy/internal/utils"
)

type (
	PortMapping = map[int]container.Port
	Container   struct {
		_ utils.NoCopy

		DockerHost    string          `json:"docker_host"`
		Image         *ContainerImage `json:"image"`
		ContainerName string          `json:"container_name"`
		ContainerID   string          `json:"container_id"`

		Agent *agent.AgentConfig `json:"agent"`

		Labels            map[string]string   `json:"-"`
		IdlewatcherConfig *idlewatcher.Config `json:"idlewatcher_config"`

		Mounts []string `json:"mounts"`

		Network            string      `json:"network,omitempty"`
		PublicPortMapping  PortMapping `json:"public_ports"`  // non-zero publicPort:types.Port
		PrivatePortMapping PortMapping `json:"private_ports"` // privatePort:types.Port
		PublicHostname     string      `json:"public_hostname"`
		PrivateHostname    string      `json:"private_hostname"`

		Aliases           []string `json:"aliases"`
		IsExcluded        bool     `json:"is_excluded"`
		IsExplicit        bool     `json:"is_explicit"`
		IsHostNetworkMode bool     `json:"is_host_network_mode"`
		Running           bool     `json:"running"`

		Errors *containerError `json:"errors"`
	}
	ContainerImage struct {
		Author string `json:"author,omitempty"`
		Name   string `json:"name"`
		Tag    string `json:"tag,omitempty"`
	}
)

var DummyContainer = new(Container)

var (
	ErrNetworkNotFound = errors.New("network not found")
	ErrNoNetwork       = errors.New("no network found")
)

func FromDocker(c *container.SummaryTrimmed, dockerHost string) (res *Container) {
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
	res = &Container{
		DockerHost:    dockerHost,
		Image:         helper.parseImage(),
		ContainerName: helper.getName(),
		ContainerID:   c.ID,

		Labels: c.Labels,

		Mounts: helper.getMounts(),

		Network:            network,
		PublicPortMapping:  helper.getPublicPortMapping(),
		PrivatePortMapping: helper.getPrivatePortMapping(),

		Aliases:           helper.getAliases(),
		IsExcluded:        isExcluded,
		IsExplicit:        isExplicit,
		IsHostNetworkMode: c.HostConfig.NetworkMode == "host",
		Running:           c.Status == "running" || c.State == "running",
	}

	if agent.IsDockerHostAgent(dockerHost) {
		var ok bool
		res.Agent, ok = agent.GetAgent(dockerHost)
		if !ok {
			res.addError(fmt.Errorf("agent %q not found", dockerHost))
		}
	}

	res.setPrivateHostname(helper)
	res.setPublicHostname()
	res.loadDeleteIdlewatcherLabels(helper)

	if res.PrivateHostname == "" && res.PublicHostname == "" && res.Running {
		res.addError(ErrNoNetwork)
	}
	return
}

func (c *Container) IsBlacklisted() bool {
	return c.Image.IsBlacklisted() || c.isDatabase()
}

func (c *Container) UpdatePorts() error {
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

func (c *Container) DockerComposeProject() string {
	return c.Labels["com.docker.compose.project"]
}

func (c *Container) DockerComposeService() string {
	return c.Labels["com.docker.compose.service"]
}

func (c *Container) Dependencies() []string {
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

func (c *Container) isDatabase() bool {
	for _, m := range c.Mounts {
		if _, ok := databaseMPs[m]; ok {
			return true
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

func (c *Container) isLocal() bool {
	if strings.HasPrefix(c.DockerHost, "unix://") {
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

func (c *Container) setPublicHostname() {
	if !c.Running {
		return
	}
	if c.isLocal() {
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

func (c *Container) setPrivateHostname(helper containerHelper) {
	if !c.isLocal() && c.Agent == nil {
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
		if proj := c.DockerComposeProject(); proj != "" {
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
		c.addError(fmt.Errorf("network %q not found, %w", c.Network, nearest))
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

func (c *Container) loadDeleteIdlewatcherLabels(helper containerHelper) {
	cfg := map[string]any{
		"idle_timeout":   helper.getDeleteLabel(LabelIdleTimeout),
		"wake_timeout":   helper.getDeleteLabel(LabelWakeTimeout),
		"stop_method":    helper.getDeleteLabel(LabelStopMethod),
		"stop_timeout":   helper.getDeleteLabel(LabelStopTimeout),
		"stop_signal":    helper.getDeleteLabel(LabelStopSignal),
		"start_endpoint": helper.getDeleteLabel(LabelStartEndpoint),
		"depends_on":     c.Dependencies(),
	}

	// ensure it's deleted from labels
	helper.getDeleteLabel(LabelDependsOn)

	// set only if idlewatcher is enabled
	idleTimeout := cfg["idle_timeout"]
	if idleTimeout != "" {
		idwCfg := new(idlewatcher.Config)
		idwCfg.Docker = &idlewatcher.DockerConfig{
			DockerHost:    c.DockerHost,
			ContainerID:   c.ContainerID,
			ContainerName: c.ContainerName,
		}

		err := serialization.MapUnmarshalValidate(cfg, idwCfg)
		if err != nil {
			c.addError(err)
		} else {
			c.IdlewatcherConfig = idwCfg
		}
	}
}

func (c *Container) addError(err error) {
	if c.Errors == nil {
		c.Errors = new(containerError)
	}
	c.Errors.Add(err)
}
