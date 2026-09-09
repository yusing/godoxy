# internal/idlewatcher/runtime

Idlewatcher configuration and provider contracts.

## Overview

`internal/idlewatcher/runtime` contains the shared types used by route config,
Docker label parsing, proxmox config, watcher providers, and the idlewatcher
runtime. The watcher implementation itself lives in `internal/idlewatcher`.

## Responsibilities

- Define idlewatcher config fields and validation.
- Define Docker/proxmox provider config references used by idlewatcher.
- Define stop method, signal, status, path, provider, and waker contracts.
- Provide defaults for wake and stop timeouts.

## Non-Goals

- No container start/stop implementation.
- No HTTP loading page or SSE event handling.
- No request/stream idle detection loop.

## Key Types

```go
type IdlewatcherConfig struct {
    IdlewatcherProviderConfig
    IdlewatcherConfigBase

    StartEndpoint string
    DependsOn     []string
    NoLoadingPage bool
}
```

```go
// on IdlewatcherConfigBase, so it survives the config copy done on reload.
type IdlewatcherNotifyConfig struct {
    Enabled *bool                    // nil: inherit defaults, then fall back to len(To) > 0
    To      []string                 // providers.notification names; empty means all
    Events  []IdlewatcherNotifyEvent // empty means NotifyEventsDefault
}
```

### Notify events

`sleep`, `wake`, `ready`, `error`, `sleep_failed`, plus `all` as a shorthand for
every event. Unknown names are rejected at deserialization time by
`IdlewatcherNotifyConfig.Validate`, which runs for YAML routes and Docker labels
alike. `NotifyEventsDefault` is `[sleep, wake]`.

Global values under `defaults.idlewatcher.notify` are merged into each route by
`internal/routevalidate.finalize`, which calls
`IdlewatcherNotifyConfig.ApplyDefaults`. A route sets `enabled` explicitly to
override the global default in either direction.

```go
type Provider interface {
    ContainerPause(ctx context.Context) error
    ContainerStart(ctx context.Context) error
    ContainerStop(ctx context.Context, signal ContainerSignal, timeout int) error
    ContainerKill(ctx context.Context, signal ContainerSignal) error
    ContainerStatus(ctx context.Context) (ContainerStatus, error)
}
```

## Consumers

- `internal/docker` parses Docker labels into `IdlewatcherConfig`.
- `internal/routevalidate` copies proxmox/container metadata into route idlewatcher config.
- `internal/idlewatcher` runs the watcher loop from this config.
- `internal/idlewatcher/provider` implements provider operations.
