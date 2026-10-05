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
// Notify is *IdlewatcherNotifyConfig on IdlewatcherConfigBase, so it survives reload.
type IdlewatcherNotifyConfig struct {
    To []string `json:"to,omitzero"` // nil: inherit or broadcast; []: disable
}
```

Global values under `defaults.idlewatcher.notify` are merged into each route by
`internal/routevalidate.finalize`, which calls
`IdlewatcherNotifyConfig.ApplyDefaults`. An absent `notify` pointer inherits the
global block, or stays nil and silent when no default exists. An empty block
(`notify: {}`) opts in; omitted `to` inherits global targets. If neither block
specifies targets, `To` stays nil and dispatch resolves all current providers at
send time, without capturing the provider list during initialization.

A non-nil empty `To` (`notify: {to: []}`) disables notifications and overrides
global targets. Global `to: []` also disables them, but a route can override it
with named providers. JSON `omitzero` preserves empty blocks and explicit `[]`.

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
