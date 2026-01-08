# Idlewatcher

Idlewatcher manages container lifecycle based on idle timeout. When a container is idle for a configured duration, it can be automatically stopped, paused, or killed. When a request comes in, the container is woken up automatically.

Idlewatcher also serves a small loading page (HTML + JS + CSS) and an SSE endpoint under [`internal/idlewatcher/types/paths.go`](internal/idlewatcher/types/paths.go:1) (prefixed with `/$godoxy/`) to provide wake events to browsers.

## Architecture Overview

```mermaid
graph TB
    subgraph Request Flow
        HTTP[HTTP Request] -->|Intercept| W[Watcher]
        Stream[Stream Request] -->|Intercept| W
    end

    subgraph Wake Process
        W -->|Wake| Wake[Wake Container]
        Wake -->|Check Status| State[Container State]
        Wake -->|Wait Ready| Health[Health Check]
        Wake -->|Events| SSE[SSE Events]
    end

    subgraph Idle Management
        Timer[Idle Timer] -->|Timeout| Stop[Stop Container]
        State -->|Running| Timer
        State -->|Stopped| Timer
    end

    subgraph Providers
        Docker[DockerProvider] --> DockerAPI[Docker API]
        Proxmox[ProxmoxProvider] --> ProxmoxAPI[Proxmox API]
    end

    W -->|Uses| Providers
```

## Directory Structure

```
idlewatcher/
├── debug.go               # Debug utilities for watcher inspection
├── errors.go              # Error types and conversion
├── events.go              # Wake event types and broadcasting
├── handle_http.go         # HTTP request handling and loading page
├── handle_http_debug.go   # Debug HTTP handler (!production builds)
├── handle_stream.go       # Stream connection handling
├── health.go              # Health monitor implementation + readiness tracking
├── loading_page.go        # Loading page HTML/CSS/JS templates
├── state.go               # Container state management
├── watcher.go             # Core Watcher implementation
├── provider/              # Container provider implementations
│   ├── docker.go          # Docker container management
│   └── proxmox.go         # Proxmox LXC management
├── types/
│   ├── container_status.go # ContainerStatus enum
│   ├── paths.go            # Loading page + SSE paths
│   ├── provider.go         # Provider interface definition
│   └── waker.go            # Waker interface (http + stream + health)
└── html/
    ├── loading_page.html  # Loading page template
    ├── style.css          # Loading page styles
    └── loading.js         # Loading page JavaScript
```

## Core Components

### Watcher

The main component that manages a single container's lifecycle:

```mermaid
classDiagram
    class Watcher {
        +string Key() string
        +Wake(ctx context.Context) error
        +Start(parent task.Parent) gperr.Error
        +ServeHTTP(rw ResponseWriter, r *Request)
        +ListenAndServe(ctx context.Context, predial, onRead HookFunc)
        -idleTicker: *time.Ticker
        -healthTicker: *time.Ticker
        -state: synk.Value~*containerState~
        -provider: synk.Value~Provider~
        -readyNotifyCh: chan struct{}
        -eventChs: *xsync.Map~chan *WakeEvent, struct{}~
        -eventHistory: []WakeEvent
        -dependsOn: []*dependency
    }

    class containerState {
        +status: ContainerStatus
        +ready: bool
        +err: error
        +startedAt: time.Time
        +healthTries: int
    }

    class dependency {
        +*Watcher
        +waitHealthy: bool
    }

    Watcher --> containerState : manages
    Watcher --> dependency : depends on
```

Package-level helpers:

- `watcherMap` is a global registry of watchers keyed by [`types.IdlewatcherConfig.Key()`](internal/types/idlewatcher.go:60), guarded by `watcherMapMu`.
- `singleFlight` is a global `singleflight.Group` keyed by container name to prevent duplicate wake calls.

### Provider Interface

Abstraction for different container backends:

```mermaid
classDiagram
    class Provider {
        <<interface>>
        +ContainerPause(ctx) error
        +ContainerUnpause(ctx) error
        +ContainerStart(ctx) error
        +ContainerStop(ctx, signal, timeout) error
        +ContainerKill(ctx, signal) error
        +ContainerStatus(ctx) (ContainerStatus, error)
        +Watch(ctx) (eventCh, errCh)
        +Close()
    }

    class DockerProvider {
        +client: *docker.SharedClient
        +watcher: watcher.DockerWatcher
        +containerID: string
    }

    class ProxmoxProvider {
        +*proxmox.Node
        +vmid: int
        +lxcName: string
        +running: bool
    }

    Provider <|-- DockerProvider
    Provider <|-- ProxmoxProvider
```

### Container Status

```mermaid
stateDiagram-v2
    [*] --> Napping: status=stopped|paused

    Napping --> Starting: provider start/unpause event
    Starting --> Ready: health check passes
    Starting --> Error: health check error / startup timeout

    Ready --> Napping: idle timeout (pause/stop/kill)
    Ready --> Error: health check error

    Error --> Napping: provider stop/pause event
    Error --> Starting: provider start/unpause event
```

Implementation notes:

- `Starting` is represented by `containerState{status: running, ready: false, startedAt: non-zero}`.
- `Ready` is represented by `containerState{status: running, ready: true}`.
- `Error` is represented by `containerState{status: error, err: non-nil}`.
- State is updated primarily from provider events in [`(*Watcher).watchUntilDestroy()`](internal/idlewatcher/watcher.go:553) and health checks in [`(*Watcher).checkUpdateState()`](internal/idlewatcher/health.go:104).

## Lifecycle Flow

### Wake Flow (HTTP)

```mermaid
sequenceDiagram
    participant C as Client
    participant W as Watcher
    participant P as Provider
    participant SSE as SSE (/\$godoxy/wake-events)

    C->>W: HTTP Request
    W->>W: resetIdleTimer()
    Note over W: Handles /favicon.ico and /\$godoxy/* assets first

    alt Container already ready
        W->>C: Reverse-proxy upstream (same request)
    else
        W->>W: Wake() (singleflight + deps)

        alt Non-HTML request OR NoLoadingPage=true
            W->>C: 100 Continue
            W->>W: waitForReady() (readyNotifyCh)
            W->>C: Reverse-proxy upstream (same request)
        else HTML + loading page
            W->>C: Serve loading page (HTML)
            C->>SSE: Connect (EventSource)
            Note over SSE: Streams history + live wake events
            C->>W: Retry original request when WakeEventReady
        end
    end
```

### Stream Wake Flow

```mermaid
sequenceDiagram
    participant C as Client
    participant W as Watcher

    C->>W: Connect to stream
    W->>W: preDial hook
    W->>W: wakeFromStream()
    alt Container ready
        W->>W: Pass through
    else
        W->>W: Wake() (singleflight + deps)
        W->>W: waitStarted() (wait for route to be started)
        W->>W: waitForReady() (readyNotifyCh)
        W->>C: Stream connected
    end
```

### Idle Timeout Flow

```mermaid
sequenceDiagram
    participant Client as Client
    participant T as Idle Timer
    participant W as Watcher
    participant P as Provider
    participant D as Dependencies

    loop Every request
        Client->>W: HTTP/Stream
        W->>W: resetIdleTimer()
    end

    T->>W: Timeout
    W->>W: stopByMethod()
    alt stop method = pause
        W->>P: ContainerPause()
    else stop method = stop
        W->>P: ContainerStop(signal, timeout)
    else kill method = kill
        W->>P: ContainerKill(signal)
    end
    P-->>W: Result
    W->>D: Stop dependencies
    D-->>W: Done
```

## Dependency Management

Watchers can depend on other containers being started first:

```mermaid
graph LR
    A[App] -->|depends on| B[Database]
    A -->|depends on| C[Redis]
    B -->|depends on| D[Cache]
```

```mermaid
sequenceDiagram
    participant A as App Watcher
    participant B as DB Watcher
    participant P as Provider

    A->>B: Wake()
    Note over B: SingleFlight prevents<br/>duplicate wake
    B->>P: ContainerStart()
    P-->>B: Started
    B->>B: Wait healthy
    B-->>A: Ready
    A->>P: ContainerStart()
    P-->>A: Started
```

## Event System

Wake events are broadcast via Server-Sent Events (SSE):

```mermaid
classDiagram
    class WakeEvent {
        +Type: WakeEventType
        +Message: string
        +Timestamp: time.Time
        +Error: string
        +WriteSSE(w io.Writer) error
    }

    class WakeEventType {
        <<enumeration>>
        WakeEventStarting
        WakeEventWakingDep
        WakeEventDepReady
        WakeEventContainerWoke
        WakeEventWaitingReady
        WakeEventReady
        WakeEventError
    }

    WakeEvent --> WakeEventType
```

Notes:

- The SSE endpoint is [`idlewatcher.WakeEventsPath`](internal/idlewatcher/types/paths.go:3).
- Each SSE subscriber gets a dedicated buffered channel; the watcher also keeps an in-memory `eventHistory` that is sent to new subscribers first.
- `eventHistory` is cleared when the container transitions to napping (stop/pause).

## State Machine

```mermaid
stateDiagram-v2
    Napping --> Starting: provider start/unpause event
    Starting --> Ready: Health check passes
    Starting --> Error: Health check fails / startup timeout
    Error --> Napping: provider stop/pause event
    Error --> Starting: provider start/unpause event
    Ready --> Napping: Idle timeout
    Ready --> Napping: Manual stop

    note right of Napping
        Container is stopped or paused
        Idle timer stopped
    end note

    note right of Starting
        Container is running but not ready
        Health checking active
        Events broadcasted
    end note

    note right of Ready
        Container healthy
        Idle timer running
    end note
```

## Key Files

| File                  | Purpose                                               |
| --------------------- | ----------------------------------------------------- |
| `watcher.go`          | Core Watcher implementation with lifecycle management |
| `handle_http.go`      | HTTP interception and loading page serving            |
| `handle_stream.go`    | Stream connection wake handling                       |
| `provider/docker.go`  | Docker container operations                           |
| `provider/proxmox.go` | Proxmox LXC container operations                      |
| `state.go`            | Container state transitions                           |
| `events.go`           | Event broadcasting via SSE                            |
| `health.go`           | Health monitor implementation + readiness tracking    |

## Configuration

See [`types.IdlewatcherConfig`](internal/types/idlewatcher.go:27) for configuration options:

- `IdleTimeout`: Duration before container is put to sleep
- `StopMethod`: pause, stop, or kill
- `StopSignal`: Signal to send when stopping
- `StopTimeout`: Timeout for stop operation
- `WakeTimeout`: Timeout for wake operation
- `DependsOn`: List of dependent containers
- `StartEndpoint`: Optional HTTP path restriction for wake requests
- `NoLoadingPage`: Skip loading page, wait directly

Provider config (exactly one must be set):

- `Docker`: container id/name + docker connection info
- `Proxmox`: `node` + `vmid`

## Thread Safety

- Uses `synk.Value` for atomic state updates
- Uses `xsync.Map` for SSE subscriber management
- Uses `sync.RWMutex` for watcher map (`watcherMapMu`) and SSE event history (`eventHistoryMu`)
- Uses `singleflight.Group` to prevent duplicate wake calls
