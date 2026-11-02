# internal/health

Shared health configuration, status, and monitor contracts.

## Overview

`internal/health` owns the data model for health checking. Protocol-specific
checks and monitor scheduling are implemented in subpackages.

## Responsibilities

- Define `HealthCheckConfig` and default interval/timeout/retry/silence behavior.
- Define health status values and JSON encoding.
- Define monitor/checker interfaces used by routes, load balancers, idlewatchers, and APIs.
- Define health JSON/API representation structs.

## Non-Goals

- No protocol-specific checks. Use `internal/health/check`.
- No monitor loop or notification scheduling. Use `internal/health/monitor`.
- No route selection or entrypoint logic.

## Subpackages

- `check`: low-level HTTP, H2C, Docker, file-server, and stream health checks.
- `monitor`: monitor construction, scheduling, last-seen tracking, and notifications.

## Key Types

```go
type HealthCheckConfig struct {
    Disable  bool
    UseGet   bool
    Path     string
    Interval time.Duration
    Timeout  time.Duration
    Retries  int64
    Silence  []SilenceWindow
}
```

`Silence` holds daily `HH:MM-HH:MM` windows in container local time (`TZ`),
possibly crossing midnight, during which service-down notifications are
deferred. Windows compare wall-clock time, so on a daylight saving change a
window inside the skipped hour never matches. `ApplyDefaults` inherits it only
when nil, so a route can opt out of the global `defaults.healthcheck.silence`
with an empty list (`"[]"` as a Docker label value).

```go
type HealthMonitor interface {
    task.TaskStarter
    task.TaskFinisher
    fmt.Stringer
    WithHealthInfo
    Name() string
    json.Marshaler
}
```
