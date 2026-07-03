# internal/loadbalancer

Serializable load-balancer configuration.

## Overview

`internal/loadbalancer` contains the config types used in route files, Docker
labels, Swagger output, and the HTTP load-balancer runtime. It deliberately does
not implement request distribution; that lives in `internal/net/gphttp/loadbalancer`.

## Responsibilities

- Define route config for load-balanced HTTP routes.
- Define supported balancing mode values.
- Normalize mode names during validation.
- Provide sticky-session defaults shared by config and runtime.

## Non-Goals

- No HTTP request handling.
- No backend server pool.
- No health monitor implementation.

## API

```go
type Config struct {
    Link         string
    Mode         Mode
    Weight       int
    Sticky       bool
    StickyMaxAge time.Duration
    Options      map[string]any
}
```

```go
const (
    ModeUnset      Mode = ""
    ModeRoundRobin Mode = "roundrobin"
    ModeLeastConn  Mode = "leastconn"
    ModeIPHash     Mode = "iphash"
)
```

Use `internal/net/gphttp/loadbalancer` for the runtime `LoadBalancer` and server pool.
