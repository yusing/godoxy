package docker

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/yusing/go-proxy/agent/pkg/agent"
	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/gperr"
	idlewatcher "github.com/yusing/go-proxy/internal/idlewatcher/types"
	"github.com/yusing/go-proxy/internal/logging"
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

		PublicPortMapping  PortMapping `json:"public_ports"`  // non-zero publicPort:types.Port
		PrivatePortMapping PortMapping `json:"private_ports"` // privatePort:types.Port
		PublicHostname     string      `json:"public_hostname"`
		PrivateHostname    string      `json:"private_hostname"`

		Aliases           []string `json:"aliases"`
		IsExcluded        bool     `json:"is_excluded"`
		IsExplicit        bool     `json:"is_explicit"`
		IsHostNetworkMode bool     `json:"is_host_network_mode"`
		Running           bool     `json:"running"`
	}
	ContainerImage struct {
		Author string `json:"author,omitempty"`
		Name   string `json:"name"`
		Tag    string `json:"tag,omitempty"`
	}
)

var DummyContainer = new(Container)

func FromDocker(c *container.SummaryTrimmed, dockerHost string) (res *Container) {
	isExplicit := false
	helper := containerHelper{c}
	for lbl := range c.Labels {
		if strings.HasPrefix(lbl, NSProxy+".") {
			isExplicit = true
		} else {
			delete(c.Labels, lbl)
		}
	}

	isExcluded, _ := strconv.ParseBool(helper.getDeleteLabel(LabelExclude))
	res = &Container{
		DockerHost:    dockerHost,
		Image:         helper.parseImage(),
		ContainerName: helper.getName(),
		ContainerID:   c.ID,

		Labels: c.Labels,

		Mounts: helper.getMounts(),

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
		res.Agent, ok = config.GetInstance().GetAgent(dockerHost)
		if !ok {
			logging.Error().Msgf("agent %q not found", dockerHost)
		}
	}

	res.setPrivateHostname(helper)
	res.setPublicHostname()
	res.loadDeleteIdlewatcherLabels(helper)
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
			PublicPort:  uint16(portInt),
			PrivatePort: uint16(portInt),
			Type:        proto,
		}
	}
	return nil
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
	switch url.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
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
		logging.Err(err).Msgf("invalid docker host %q, falling back to 127.0.0.1", c.DockerHost)
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
	for _, v := range helper.NetworkSettings.Networks {
		if v.IPAddress != "" {
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
	}
	// set only if idlewatcher is enabled
	idleTimeout := cfg["idle_timeout"]
	if idleTimeout != "" {
		idwCfg := &idlewatcher.Config{
			Docker: &idlewatcher.DockerConfig{
				DockerHost:    c.DockerHost,
				ContainerID:   c.ContainerID,
				ContainerName: c.ContainerName,
			},
		}
		err := utils.MapUnmarshalValidate(cfg, idwCfg)
		if err != nil {
			gperr.LogWarn("invalid idlewatcher config", gperr.PrependSubject(c.ContainerName, err))
		} else {
			c.IdlewatcherConfig = idwCfg
		}
	}
}
