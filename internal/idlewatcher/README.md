# Idlewatcher

Idlewatcher manages container lifecycle based on idle timeout. When a container is idle for a configured duration, it can be automatically stopped, paused, or killed. When a request comes in, the container is woken up automatically.

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
├── cmd                    # Command execution utilities
├── debug.go               # Debug utilities for watcher inspection
├── errors.go              # Error types and conversion
├── events.go              # Wake event types and broadcasting
├── handle_http.go         # HTTP request handling and loading page
├── handle_http_debug.go   # Debug HTTP handler (dev only)
├── handle_stream.go       # Stream connection handling
├── health.go              # Health monitoring interface
├── loading_page.go        # Loading page HTML/CSS/JS templates
├── state.go               # Container state management
├── watcher.go             # Core Watcher implementation
├── provider/              # Container provider implementations
│   ├── docker.go          # Docker container management
│   └── proxmox.go         # Proxmox LXC management
├── types/
│   └── provider.go        # Provider interface definition
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
    [*] --> Napping: Container stopped/paused
    Napping --> Waking: Wake request
    Waking --> Running: Container started
    Running --> Starting: Container is running but not healthy
    Starting --> Ready: Health check passes
    Ready --> Napping: Idle timeout
    Ready --> Error check fails: Health
    Error --> Waking: Retry wake
```

## Lifecycle Flow

### Wake Flow (HTTP)

```mermaid
sequenceDiagram
    participant C as Client
    participant W as Watcher
    participant P as Provider
    participant H as HealthChecker
    participant SSE as SSE Events

    C->>W: HTTP Request
    W->>W: resetIdleTimer()
    alt Container already ready
        W->>W: return true (proceed)
    else
        alt No loading page configured
            W->>P: ContainerStart()
            W->>H: Wait for healthy
            H-->>W: Healthy
            W->>C: Continue request
        else Loading page enabled
            W->>P: ContainerStart()
            W->>SSE: Send WakeEventStarting
            W->>C: Serve loading page
            loop Health checks
                H->>H: Check health
                H-->>W: Not healthy yet
                W->>SSE: Send progress
            end
            H-->>W: Healthy
            W->>SSE: Send WakeEventReady
            C->>W: SSE connection
            W->>SSE: Events streamed
            C->>W: Poll/retry request
            W->>W: return true (proceed)
        end
    end
```

### Stream Wake Flow

```mermaid
sequenceDiagram
    participant C as Client
    participant W as Watcher
    participant P as Provider
    participant H as HealthChecker

    C->>W: Connect to stream
    W->>W: preDial hook
    W->>W: wakeFromStream()
    alt Container ready
        W->>W: Pass through
    else
        W->>P: ContainerStart()
        W->>W: waitStarted()
        W->>H: Wait for healthy
        H-->>W: Healthy
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

## State Machine

```mermaid
stateDiagram-v2
    note right of Napping
        Container is stopped or paused
        Idle timer stopped
    end note

    note right of Waking
        Container is starting
        Health checking active
        Events broadcasted
    end note

    note right of Ready
        Container healthy
        Idle timer running
    end note

    Napping --> Waking: Wake()
    Waking --> Ready: Health check passes
    Waking --> Error: Health check fails
    Error --> Waking: Retry
    Ready --> Napping: Idle timeout
    Ready --> Napping: Manual stop
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
| `health.go`           | Health monitor interface implementation               |

## Configuration

See `types.IdlewatcherConfig` for configuration options:

- `IdleTimeout`: Duration before container is put to sleep
- `StopMethod`: pause, stop, or kill
- `StopSignal`: Signal to send when stopping
- `StopTimeout`: Timeout for stop operation
- `WakeTimeout`: Timeout for wake operation
- `DependsOn`: List of dependent containers
- `StartEndpoint`: Optional endpoint restriction for wake requests
- `NoLoadingPage`: Skip loading page, wait directly

## Thread Safety

- Uses `synk.Value` for atomic state updates
- Uses `xsync.Map` for SSE subscriber management
- Uses `sync.RWMutex` for watcher map access
- Uses `singleflight.Group` to prevent duplicate wake calls
