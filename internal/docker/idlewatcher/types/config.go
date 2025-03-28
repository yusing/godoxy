package types

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/yusing/go-proxy/internal/docker"
	"github.com/yusing/go-proxy/internal/gperr"
)

type (
	Config struct {
		IdleTimeout   time.Duration `json:"idle_timeout,omitempty"`
		WakeTimeout   time.Duration `json:"wake_timeout,omitempty"`
		StopTimeout   int           `json:"stop_timeout,omitempty"` // docker api takes integer seconds for timeout argument
		StopMethod    StopMethod    `json:"stop_method,omitempty"`
		StopSignal    Signal        `json:"stop_signal,omitempty"`
		StartEndpoint string        `json:"start_endpoint,omitempty"` // Optional path that must be hit to start container
	}
	StopMethod string
	Signal     string
)

const (
	StopMethodPause StopMethod = "pause"
	StopMethodStop  StopMethod = "stop"
	StopMethodKill  StopMethod = "kill"
)

var validSignals = map[string]struct{}{
	"":       {},
	"SIGINT": {}, "SIGTERM": {}, "SIGHUP": {}, "SIGQUIT": {},
	"INT": {}, "TERM": {}, "HUP": {}, "QUIT": {},
}

func ValidateConfig(cont *docker.Container) (*Config, gperr.Error) {
	if cont == nil || cont.IdleTimeout == "" {
		return nil, nil
	}

	errs := gperr.NewBuilder("invalid idlewatcher config")

	idleTimeout := gperr.Collect(errs, validateDurationPostitive, cont.IdleTimeout)
	wakeTimeout := gperr.Collect(errs, validateDurationPostitive, cont.WakeTimeout)
	stopTimeout := gperr.Collect(errs, validateDurationPostitive, cont.StopTimeout)
	stopMethod := gperr.Collect(errs, validateStopMethod, cont.StopMethod)
	signal := gperr.Collect(errs, validateSignal, cont.StopSignal)
	startEndpoint := gperr.Collect(errs, validateStartEndpoint, cont.StartEndpoint)

	if errs.HasError() {
		return nil, errs.Error()
	}

	return &Config{
		IdleTimeout:   idleTimeout,
		WakeTimeout:   wakeTimeout,
		StopTimeout:   int(stopTimeout.Seconds()),
		StopMethod:    stopMethod,
		StopSignal:    signal,
		StartEndpoint: startEndpoint,
	}, nil
}

func validateDurationPostitive(value string) (time.Duration, error) {
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	if d < 0 {
		return 0, errors.New("duration must be positive")
	}
	return d, nil
}

func validateSignal(s string) (Signal, error) {
	if _, ok := validSignals[s]; ok {
		return Signal(s), nil
	}
	return "", errors.New("invalid signal " + s)
}

func validateStopMethod(s string) (StopMethod, error) {
	sm := StopMethod(s)
	switch sm {
	case StopMethodPause, StopMethodStop, StopMethodKill:
		return sm, nil
	default:
		return "", errors.New("invalid stop method " + s)
	}
}

func validateStartEndpoint(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	// checks needed as of Go 1.6 because of change https://github.com/golang/go/commit/617c93ce740c3c3cc28cdd1a0d712be183d0b328#diff-6c2d018290e298803c0c9419d8739885L195
	// emulate browser and strip the '#' suffix prior to validation. see issue-#237
	if i := strings.Index(s, "#"); i > -1 {
		s = s[:i]
	}
	if len(s) == 0 {
		return "", errors.New("start endpoint must not be empty if defined")
	}
	if _, err := url.ParseRequestURI(s); err != nil {
		return "", err
	}
	return s, nil
}
