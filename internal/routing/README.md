# internal/routing

Shared routing contracts.

## Overview

`internal/routing` defines the interfaces that decouple route configuration,
route implementations, providers, entrypoints, middleware, health, and API code.
It contains no concrete route implementation and no listener logic.

## Responsibilities

- Define `Route`, `HTTPRoute`, `ReverseProxyRoute`, `FileServerRoute`, and `StreamRoute`.
- Define provider and entrypoint interfaces used across packages.
- Provide lightweight pool interfaces for route lookup and iteration.
- Define provider type and route/provider statistics types.
- Store and retrieve an entrypoint from task/request contexts.

## Non-Goals

- No validation or finalization.
- No reverse proxy, file server, TCP, or UDP implementation.
- No route provider implementation.
- No HTTP server or stream listener implementation.

## Key Interfaces

```go
type Route interface {
    task.TaskStarter
    task.TaskFinisher
    pool.Object
    zerolog.LogObjectMarshaler

    ProviderName() string
    ListenURL() *nettypes.URL
    TargetURL() *nettypes.URL
    HealthMonitor() health.HealthMonitor
    ShouldExclude() bool
    Started() <-chan struct{}
}
```

Specialized route interfaces add behavior:

- `HTTPRoute` embeds `http.Handler`.
- `ReverseProxyRoute` exposes `ReverseProxy()`.
- `FileServerRoute` exposes `RootPath()`.
- `StreamRoute` embeds `nettypes.Stream`.

## Entrypoint Context

Task and request contexts store the active entrypoint using `EntrypointContextKey`.
Callers should use the helpers instead of setting the key directly:

```go
routing.SetEntrypointCtx(task, ep)
ep := routing.EntrypointFromCtx(ctx)
```

The `internal/entrypoint` package re-exports these helpers as a compatibility
shim for callers that should not import `routing` directly.

## Dependency Map

| Consumer | Uses |
| --- | --- |
| `internal/route` | Stores concrete implementations as `routing.Route` |
| `internal/routeimpl` | Implements route interfaces |
| `internal/entrypoint` | Implements `routing.Entrypoint` |
| `internal/route/provider` | Implements `routing.Provider` |
| Middleware/rules/API | Lookup routes and entrypoint state without concrete imports |
