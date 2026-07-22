package config

import (
	"github.com/rs/zerolog"
	"github.com/yusing/godoxy/internal/proxmox"
)

// LoadDiagnostics is implemented by a configuration state that owns buffered
// diagnostics for its load attempt. Callers must retain a process-log fallback
// for standalone validation with another State implementation.
type LoadDiagnostics interface {
	LoadLogger() *zerolog.Logger
	LogProxmoxDiscoveries([]proxmox.Discovery)
}
