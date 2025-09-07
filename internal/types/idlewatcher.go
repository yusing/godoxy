package types

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yusing/go-proxy/internal/gperr"
)

type (
	IdlewatcherProviderConfig struct {
		Proxmox *ProxmoxConfig `json:"proxmox,omitempty"`
		Docker  *DockerConfig  `json:"docker,omitempty"`
	} // @name IdlewatcherProviderConfig
	IdlewatcherConfigBase struct {
		// 0: no idle watcher.
		// Positive: idle watcher with idle timeout.
		// Negative: idle watcher as a dependency.	IdleTimeout time.Duration `json:"idle_timeout" json_ext:"duration"`
		IdleTimeout time.Duration       `json:"idle_timeout"`
		WakeTimeout time.Duration       `json:"wake_timeout"`
		StopTimeout time.Duration       `json:"stop_timeout"`
		StopMethod  ContainerStopMethod `json:"stop_method"`
		StopSignal  ContainerSignal     `json:"stop_signal,omitempty"`
	} // @name IdlewatcherConfigBase
	IdlewatcherConfig struct {
		IdlewatcherProviderConfig
		IdlewatcherConfigBase

		StartEndpoint string   `json:"start_endpoint,omitempty"` // Optional path that must be hit to start container
		DependsOn     []string `json:"depends_on,omitempty"`

		valErr gperr.Error
	} // @name IdlewatcherConfig
	ContainerStopMethod string // @name ContainerStopMethod
	ContainerSignal     string // @name ContainerSignal

	DockerConfig struct {
		DockerHost    string `json:"docker_host" validate:"required"`
		ContainerID   string `json:"container_id" validate:"required"`
		ContainerName string `json:"container_name" validate:"required"`
	} // @name DockerConfig
	ProxmoxConfig struct {
		Node string `json:"node" validate:"required"`
		VMID int    `json:"vmid" validate:"required"`
	} // @name ProxmoxConfig
)

const (
	ContainerWakeTimeoutDefault = 30 * time.Second
	ContainerStopTimeoutDefault = 1 * time.Minute

	ContainerStopMethodPause ContainerStopMethod = "pause"
	ContainerStopMethodStop  ContainerStopMethod = "stop"
	ContainerStopMethodKill  ContainerStopMethod = "kill"
)

func (c *IdlewatcherConfig) Key() string {
	if c.Docker != nil {
		return c.Docker.ContainerID
	}
	return c.Proxmox.Node + ":" + strconv.Itoa(c.Proxmox.VMID)
}

func (c *IdlewatcherConfig) ContainerName() string {
	if c.Docker != nil {
		return c.Docker.ContainerName
	}
	return "lxc-" + strconv.Itoa(c.Proxmox.VMID)
}

func (c *IdlewatcherConfig) Validate() gperr.Error {
	if c.IdleTimeout == 0 { // zero idle timeout means no idle watcher
		c.valErr = nil
		return nil
	}
	errs := gperr.NewBuilder()
	errs.AddRange(
		c.validateProvider(),
		c.validateTimeouts(),
		c.validateStopMethod(),
		c.validateStopSignal(),
		c.validateStartEndpoint(),
	)
	c.valErr = errs.Error()
	return c.valErr
}

func (c *IdlewatcherConfig) ValErr() gperr.Error {
	return c.valErr
}

func (c *IdlewatcherConfig) validateProvider() error {
	if c.Docker == nil && c.Proxmox == nil {
		return gperr.New("missing idlewatcher provider config")
	}
	return nil
}

func (c *IdlewatcherConfig) validateTimeouts() error { //nolint:unparam
	if c.WakeTimeout == 0 {
		c.WakeTimeout = ContainerWakeTimeoutDefault
	}
	if c.StopTimeout == 0 {
		c.StopTimeout = ContainerStopTimeoutDefault
	}
	return nil
}

func (c *IdlewatcherConfig) validateStopMethod() error {
	switch c.StopMethod {
	case "":
		c.StopMethod = ContainerStopMethodStop
		return nil
	case ContainerStopMethodPause, ContainerStopMethodStop, ContainerStopMethodKill:
		return nil
	default:
		return gperr.New("invalid stop method").Subject(string(c.StopMethod))
	}
}

func (c *IdlewatcherConfig) validateStopSignal() error {
	switch c.StopSignal {
	case "", "SIGINT", "SIGTERM", "SIGQUIT", "SIGHUP", "INT", "TERM", "QUIT", "HUP":
		return nil
	default:
		return gperr.New("invalid stop signal").Subject(string(c.StopSignal))
	}
}

func (c *IdlewatcherConfig) validateStartEndpoint() error {
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
