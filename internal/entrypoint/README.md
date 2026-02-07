# Entrypoint

The entrypoint package provides the main HTTP entry point for GoDoxy, handling domain-based routing, middleware application, short link matching, access logging, and HTTP/TCP/UDP server lifecycle management.

## Overview

The entrypoint package implements the primary HTTP handler that receives all incoming requests, manages the lifecycle of HTTP/TCP/UDP servers, determines the target route based on hostname, applies middleware, and forwards requests to the appropriate route handler.

### Key Features

- Domain-based route lookup with subdomain support
- Short link (`go/<alias>` domain) handling
- Middleware chain application
- Access logging for all requests
- Configurable not-found handling
- Per-domain route resolution
- Multi-protocol server management (HTTP/HTTPS/TCP/UDP)
- Route pool abstractions via [`PoolLike`](internal/entrypoint/types/entrypoint.go:27) and [`RWPoolLike`](internal/entrypoint/types/entrypoint.go:33) interfaces

### Primary Consumers

- **HTTP servers**: Per-listen-addr servers dispatch requests to routes
- **Route providers**: Register routes via [`AddRoute`](internal/entrypoint/routes.go:48)
- **Configuration layer**: Validates and applies middleware/access-logging config

### Non-goals

- Does not implement route discovery (delegates to providers)
- Does not handle TLS certificate management (delegates to autocert)
- Does not implement health checks (delegates to `internal/health/monitor`)

### Stability

Internal package with stable core interfaces. The [`Entrypoint`](internal/entrypoint/types/entrypoint.go:7) interface is the public contract.

## Public API

### Entrypoint Interface

```go
type Entrypoint interface {
    // Server capabilities
    SupportProxyProtocol() bool
    DisablePoolsLog(v bool)

    // Route registry access
    GetRoute(alias string) (types.Route, bool)
    AddRoute(r types.Route)
    IterRoutes(yield func(r types.Route) bool)
    NumRoutes() int
    RoutesByProvider() map[string][]types.Route

    // Route pool accessors
    HTTPRoutes() PoolLike[types.HTTPRoute]
    StreamRoutes() PoolLike[types.StreamRoute]
    ExcludedRoutes() RWPoolLike[types.Route]

    // Health info queries
    GetHealthInfo() map[string]types.HealthInfo
    GetHealthInfoWithoutDetail() map[string]types.HealthInfoWithoutDetail
    GetHealthInfoSimple() map[string]types.HealthStatus
}
```

### Pool Interfaces

```go
type PoolLike[Route types.Route] interface {
    Get(alias string) (Route, bool)
    Iter(yield func(alias string, r Route) bool)
    Size() int
}

type RWPoolLike[Route types.Route] interface {
    PoolLike[Route]
    Add(r Route)
    Del(r Route)
}
```

### Configuration

```go
type Config struct {
    SupportProxyProtocol bool `json:"support_proxy_protocol"`
}
```

## Architecture

### Core Components

```mermaid
classDiagram
    class Entrypoint {
        +task *task.Task
        +cfg *Config
        +middleware *middleware.Middleware
        +shortLinkMatcher *ShortLinkMatcher
        +streamRoutes *pool.Pool[types.StreamRoute]
        +excludedRoutes *pool.Pool[types.Route]
        +servers *xsync.Map[string, *httpServer]
        +tcpListeners *xsync.Map[string, net.Listener]
        +udpListeners *xsync.Map[string, net.PacketConn]
        +SupportProxyProtocol() bool
        +AddRoute(r)
        +IterRoutes(yield)
        +HTTPRoutes() PoolLike
    }

    class httpServer {
        +routes *routePool
        +ServeHTTP(w, r)
        +AddRoute(route)
        +DelRoute(route)
    }

    class routePool {
        +Get(alias) (HTTPRoute, bool)
        +AddRoute(route)
        +DelRoute(route)
    }

    class PoolLike {
        <<interface>>
        +Get(alias) (Route, bool)
        +Iter(yield) bool
        +Size() int
    }

    class RWPoolLike {
        <<interface>>
        +PoolLike
        +Add(r Route)
        +Del(r Route)
    }

    Entrypoint --> httpServer : manages
    Entrypoint --> routePool : HTTPRoutes()
    Entrypoint --> PoolLike : returns
    Entrypoint --> RWPoolLike : ExcludedRoutes()
```

### Request Processing Pipeline

```mermaid
flowchart TD
    A[HTTP Request] --> B[Find Route by Host]
    B --> C{Route Found?}
    C -->|Yes| D{Middleware?}
    C -->|No| E{Short Link?}
    E -->|Yes| F[Short Link Handler]
    E -->|No| G{Not Found Handler?}
    G -->|Yes| H[Not Found Handler]
    G -->|No| I[Serve 404]

    D -->|Yes| J[Apply Middleware Chain]
    D -->|No| K[Direct Route Handler]
    J --> K

    K --> L[Route ServeHTTP]
    L --> M[Response]

    F --> M
    H --> N[404 Response]
    I --> N
```

### Server Lifecycle

```mermaid
stateDiagram-v2
    [*] --> Empty: NewEntrypoint()

    Empty --> Listening: AddRoute()
    Listening --> Listening: AddRoute()
    Listening --> Listening: delHTTPRoute()
    Listening --> [*]: Cancel()

    Listening --> AddingServer: addHTTPRoute()
    AddingServer --> Listening: Server starts

    note right of Listening
        servers map: addr -> httpServer
        tcpListeners map: addr -> Listener
        udpListeners map: addr -> PacketConn
    end note
```

## Data Flow

```mermaid
sequenceDiagram
    participant Client
    participant httpServer
    participant Entrypoint
    participant Middleware
    participant Route

    Client->>httpServer: GET /path
    httpServer->>Entrypoint: FindRoute(host)

    alt Route Found
        Entrypoint-->>httpServer: HTTPRoute
        httpServer->>Middleware: ServeHTTP(routeHandler)
        alt Has Middleware
            Middleware->>Middleware: Process Chain
        end
        Middleware->>Route: Forward Request
        Route-->>Middleware: Response
        Middleware-->>httpServer: Response
    else Short Link
        httpServer->>ShortLinkMatcher: Match short code
        ShortLinkMatcher-->>httpServer: Redirect
    else Not Found
        httpServer->>NotFoundHandler: Serve 404
        NotFoundHandler-->>httpServer: 404 Page
    end

    httpServer-->>Client: Response
```

## Route Registry

Routes are now managed per-entrypoint instead of global registry:

```go
// Adding a route
ep.AddRoute(route)

// Iterating all routes
ep.IterRoutes(func(r types.Route) bool {
    log.Info().Str("alias", r.Name()).Msg("route")
    return true // continue iteration
})

// Querying by alias
route, ok := ep.GetRoute("myapp")

// Grouping by provider
byProvider := ep.RoutesByProvider()
```

## Configuration Surface

### Config Source

Environment variables and YAML config file:

```yaml
entrypoint:
  support_proxy_protocol: true
```

### Environment Variables

| Variable                       | Description                   |
| ------------------------------ | ----------------------------- |
| `PROXY_SUPPORT_PROXY_PROTOCOL` | Enable PROXY protocol support |

## Dependency and Integration Map

| Dependency                       | Purpose                    |
| -------------------------------- | -------------------------- |
| `internal/route`                 | Route types and handlers   |
| `internal/route/rules`           | Not-found rules processing |
| `internal/logging/accesslog`     | Request logging            |
| `internal/net/gphttp/middleware` | Middleware chain           |
| `internal/types`                 | Route and health types     |
| `github.com/puzpuzpuz/xsync/v4`  | Concurrent server map      |
| `github.com/yusing/goutils/pool` | Route pool implementations |
| `github.com/yusing/goutils/task` | Lifecycle management       |

## Observability

### Logs

| Level   | Context               | Description             |
| ------- | --------------------- | ----------------------- |
| `DEBUG` | `route`, `listen_url` | Route addition/removal  |
| `DEBUG` | `addr`, `proto`       | Server lifecycle        |
| `ERROR` | `route`, `listen_url` | Server startup failures |

### Metrics

Route metrics exposed via [`GetHealthInfo`](internal/entrypoint/query.go:10) methods:

```go
// Health info for all routes
healthMap := ep.GetHealthInfo()
// {
//   "myapp": {Status: "healthy", Uptime: 3600, Latency: 5ms},
//   "excluded-route": {Status: "unknown", Detail: "n/a"},
// }
```

## Security Considerations

- Route lookup is read-only from route pools
- Middleware chain is applied per-request
- Proxy protocol support must be explicitly enabled
- Access logger captures request metadata before processing

## Failure Modes and Recovery

| Failure               | Behavior                       | Recovery                     |
| --------------------- | ------------------------------ | ---------------------------- |
| Server bind fails     | Error logged, route not added  | Fix port/address conflict    |
| Route start fails     | Route excluded, error logged   | Fix route configuration      |
| Middleware load fails | AddRoute returns error         | Fix middleware configuration |
| Context cancelled     | All servers stopped gracefully | Restart entrypoint           |

## Usage Examples

### Basic Setup

```go
ep := entrypoint.NewEntrypoint(parent, &entrypoint.Config{
    SupportProxyProtocol: false,
})

// Configure domain matching
ep.SetFindRouteDomains([]string{".example.com", "example.com"})

// Configure middleware
err := ep.SetMiddlewares([]map[string]any{
    {"rate_limit": map[string]any{"requests_per_second": 100}},
})
if err != nil {
    return err
}

// Configure access logging
err = ep.SetAccessLogger(parent, &accesslog.RequestLoggerConfig{
    Path: "/var/log/godoxy/access.log",
})
if err != nil {
    return err
}
```

### Route Querying

```go
// Iterate all routes including excluded
for r := range ep.IterRoutes {
    log.Info().
        Str("alias", r.Name()).
        Str("provider", r.ProviderName()).
        Bool("excluded", r.ShouldExclude()).
        Msg("route")
}

// Get health info for all routes
healthMap := ep.GetHealthInfoSimple()
for alias, status := range healthMap {
    log.Info().Str("alias", alias).Str("status", string(status)).Msg("health")
}
```

### Route Addition

```go
route := &route.Route{
    Alias:  "myapp",
    Scheme: route.SchemeHTTP,
    Host:   "myapp",
    Port:   route.Port{Proxy: 80, Target: 3000},
}

ep.AddRoute(route)
```

## Context Integration

Routes can access the entrypoint from request context:

```go
// Set entrypoint in context
entrypoint.SetCtx(r.Context(), ep)

// Get entrypoint from context
if ep := entrypoint.FromCtx(r.Context()); ep != nil {
    route, ok := ep.GetRoute("alias")
}
```

## Testing Notes

- Benchmark tests in [`entrypoint_benchmark_test.go`](internal/entrypoint/entrypoint_benchmark_test.go)
- Integration tests in [`entrypoint_test.go`](internal/entrypoint/entrypoint_test.go)
- Mock route pools for unit testing
- Short link tests in [`shortlink_test.go`](internal/entrypoint/shortlink_test.go)
