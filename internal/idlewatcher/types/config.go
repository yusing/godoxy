package idlewatcher

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yusing/go-proxy/internal/gperr"
)

type (
	ProviderConfig struct {
		Proxmox *ProxmoxConfig `json:"proxmox,omitempty"`
		Docker  *DockerConfig  `json:"docker,omitempty"`
	}
	IdlewatcherConfig struct {
		// 0: no idle watcher.
		// Positive: idle watcher with idle timeout.
		// Negative: idle watcher as a dependency.	IdleTimeout time.Duration `json:"idle_timeout" json_ext:"duration"`
		IdleTimeout time.Duration `json:"idle_timeout"`
		WakeTimeout time.Duration `json:"wake_timeout"`
		StopTimeout time.Duration `json:"stop_timeout"`
		StopMethod  StopMethod    `json:"stop_method"`
		StopSignal  Signal        `json:"stop_signal,omitempty"`
	}
	Config struct {
		ProviderConfig
		IdlewatcherConfig

		StartEndpoint string   `json:"start_endpoint,omitempty"` // Optional path that must be hit to start container
		DependsOn     []string `json:"depends_on,omitempty"`
	}
	StopMethod string
	Signal     string

	DockerConfig struct {
		DockerHost    string `json:"docker_host" validate:"required"`
		ContainerID   string `json:"container_id" validate:"required"`
		ContainerName string `json:"container_name" validate:"required"`
	}
	ProxmoxConfig struct {
		Node string `json:"node" validate:"required"`
		VMID int    `json:"vmid" validate:"required"`
	}
)

const (
	WakeTimeoutDefault = 30 * time.Second
	StopTimeoutDefault = 1 * time.Minute

	StopMethodPause StopMethod = "pause"
	StopMethodStop  StopMethod = "stop"
	StopMethodKill  StopMethod = "kill"
)

func (c *Config) Key() string {
	if c.Docker != nil {
		return c.Docker.ContainerID
	}
	return c.Proxmox.Node + ":" + strconv.Itoa(c.Proxmox.VMID)
}

func (c *Config) ContainerName() string {
	if c.Docker != nil {
		return c.Docker.ContainerName
	}
	return "lxc-" + strconv.Itoa(c.Proxmox.VMID)
}

func (c *Config) Validate() gperr.Error {
	if c.IdleTimeout == 0 { // zero idle timeout means no idle watcher
		return nil
	}
	errs := gperr.NewBuilder("idlewatcher config validation error")
	errs.AddRange(
		c.validateProvider(),
		c.validateTimeouts(),
		c.validateStopMethod(),
		c.validateStopSignal(),
		c.validateStartEndpoint(),
	)
	return errs.Error()
}

func (c *Config) validateProvider() error {
	if c.Docker == nil && c.Proxmox == nil {
		return gperr.New("missing idlewatcher provider config")
	}
	return nil
}

func (c *Config) validateTimeouts() error { //nolint:unparam
	if c.WakeTimeout == 0 {
		c.WakeTimeout = WakeTimeoutDefault
	}
	if c.StopTimeout == 0 {
		c.StopTimeout = StopTimeoutDefault
	}
	return nil
}

func (c *Config) validateStopMethod() error {
	switch c.StopMethod {
	case "":
		c.StopMethod = StopMethodStop
		return nil
	case StopMethodPause, StopMethodStop, StopMethodKill:
		return nil
	default:
		return gperr.New("invalid stop method").Subject(string(c.StopMethod))
	}
}

func (c *Config) validateStopSignal() error {
	switch c.StopSignal {
	case "", "SIGINT", "SIGTERM", "SIGQUIT", "SIGHUP", "INT", "TERM", "QUIT", "HUP":
		return nil
	default:
		return gperr.New("invalid stop signal").Subject(string(c.StopSignal))
	}
}

func (c *Config) validateStartEndpoint() error {
	if c.StartEndpoint == "" {
		return nil
	}
	// checks needed as of Go 1.6 because of change https://github.com/golang/go/commit/617c93ce740c3c3cc28cdd1a0d712be183d0b328#diff-6c2d018290e298803c0c9419d8739885L195
	// emulate browser and strip the '#' suffix prior to validation. see issue-#237
	if i := strings.Index(c.StartEndpoint, "#"); i > -1 {
		c.StartEndpoint = c.StartEndpoint[:i]
	}
	if len(c.StartEndpoint) == 0 {
		return gperr.New("start endpoint must not be empty if defined")
	}
	_, err := url.ParseRequestURI(c.StartEndpoint)
	return err
}
