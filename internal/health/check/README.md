# Health Check

This package provides low-level health check implementations for different protocols and services in GoDoxy.

## Health Check Types

### Docker Health Check

Checks the health status of Docker containers using the Docker API.

**Flow:**

```mermaid
flowchart TD
    A[Docker Health Check] --> B{Docker Failures > Threshold?}
    B -->|yes| C[Return Error: Too Many Failures]
    B -->|no| D[Container Inspect API Call]
    D --> E{Inspect Successful?}
    E -->|no| F[Increment Failure Count]
    E -->|yes| G[Parse Container State]

    G --> H{Container Status}
    H -->|dead/exited/paused/restarting/removing| I[Unhealthy: Container State]
    H -->|created| J[Unhealthy: Not Started]
    H -->|running| K{Health Check Configured?}

    K -->|no| L[Return Error: No Health Check]
    K -->|yes| M[Check Health Status]
    M --> N{Health Status}
    N -->|healthy| O[Healthy]
    N -->|unhealthy| P[Unhealthy: Last Log Output]

    I --> Q[Reset Failure Count]
    J --> Q
    O --> Q
    P --> Q
```

**Key Features:**

- Intercepts Docker API responses to extract container state
- Tracks failure count with configurable threshold (3 failures)
- Supports containers with and without health check configurations
- Returns detailed error information from Docker health check logs

### HTTP Health Check

Performs HTTP/HTTPS health checks using fasthttp for optimal performance.

**Flow:**

```mermaid
flowchart TD
    A[HTTP Health Check] --> B[Create FastHTTP Request]
    B --> C[Set Headers and Method]
    C --> D[Execute Request with Timeout]
    D --> E{Request Successful?}

    E -->|no| F{Error Type}
    F -->|TLS Error| G[Healthy: TLS Error Ignored]
    F -->|Other Error| H[Unhealthy: Error Details]

    E -->|yes| I{Status Code}
    I -->|5xx| J[Unhealthy: Server Error]
    I -->|Other| K[Healthy]

    G --> L[Return Result with Latency]
    H --> L
    J --> L
    K --> L
```

**Key Features:**

- Uses fasthttp for high-performance HTTP requests
- Supports both GET and HEAD methods
- Configurable timeout and path
- Handles TLS certificate verification errors gracefully
- Returns latency measurements

### H2C Health Check

Performs HTTP/2 cleartext (h2c) health checks for services that support HTTP/2 without TLS.

**Flow:**

```mermaid
flowchart TD
    A[H2C Health Check] --> B[Create HTTP/2 Transport]
    B --> C[Set AllowHTTP: true]
    C --> D[Create HTTP Request]
    D --> E[Set Headers and Method]
    E --> F[Execute Request with Timeout]
    F --> G{Request Successful?}

    G -->|no| H[Unhealthy: Error Details]
    G -->|yes| I[Check Status Code]
    I --> J{Status Code}
    J -->|5xx| K[Unhealthy: Server Error]
    J -->|Other| L[Healthy]

    H --> M[Return Result with Latency]
    K --> M
    L --> M
```

**Key Features:**

- Uses HTTP/2 transport with cleartext support
- Supports both GET and HEAD methods
- Configurable timeout and path
- Returns latency measurements

### FileServer Health Check

Checks if a file server root directory exists and is accessible.

**Flow:**

```mermaid
flowchart TD
    A[FileServer Health Check] --> B[Start Timer]
    B --> C[Stat Directory Path]
    C --> D{Directory Exists?}

    D -->|no| E[Unhealthy: Path Not Found]
    D -->|yes| F[Healthy: Directory Accessible]
    D -->|error| G[Return Error]

    E --> H[Return Result with Latency]
    F --> H
    G --> I[Return Error]
```

**Key Features:**

- Simple directory existence check
- Measures latency of filesystem operation
- Distinguishes between "not found" and other errors
- Returns detailed error information

### Stream Health Check

Checks stream endpoint connectivity by attempting to establish a network connection.

**Flow:**

```mermaid
flowchart TD
    A[Stream Health Check] --> B[Create Dialer]
    B --> C[Set Timeout and Fallback Delay]
    C --> D[Start Timer]
    D --> E[Dial Network Connection]
    E --> F{Connection Successful?}

    F -->|no| G{Error Type}
    G -->|Connection Errors| H[Unhealthy: Connection Failed]
    G -->|Other Error| I[Return Error]

    F -->|yes| J[Close Connection]
    J --> K[Healthy: Connection Established]

    H --> L[Return Result with Latency]
    K --> L
```

**Key Features:**

- Generic network connection check
- Supports any stream protocol (TCP, UDP, etc.)
- Handles common connection errors gracefully
- Measures connection establishment latency
- Automatically closes connections

## Common Features

### Error Handling

All health checks implement consistent error handling:

- **Temporary Errors**: Network timeouts, connection failures
- **Permanent Errors**: Invalid configurations, missing resources
- **Graceful Degradation**: Returns health status even when errors occur

### Performance Monitoring

- **Latency Measurement**: All checks measure execution time
- **Timeout Support**: Configurable timeouts prevent hanging
- **Resource Cleanup**: Proper cleanup of connections and resources

### Integration

These health checks are used by the monitor package to implement route-specific health monitoring:

- HTTP/HTTPS routes use HTTP health checks
- File server routes use FileServer health checks
- Stream routes use Stream health checks
- Docker containers use Docker health checks with fallbacks
