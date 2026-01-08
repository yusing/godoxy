# godoxy.md

## Project Overview

GoDoxy is a lightweight reverse proxy with WebUI written in Go. It automatically configures routes for Docker containers using labels and provides SSL certificate management, access control, and monitoring capabilities.

## Development Commands

You should not run any commands to build or run the project.

## Documentation

If `README.md` exists in the package:

- Read it to understand what the package does and how it works.
- Update it with the changes you made (if any).

Update the roort `README.md` if relevant.

## Architecture

### Core Components

1. **Main Entry Point** (`cmd/main.go`)

   - Initializes configuration and logging
   - Starts the proxy server and WebUI

2. **Internal Packages** (`internal/`)

   - `config/` - Configuration management with YAML files
   - `route/` - HTTP routing and middleware management
   - `docker/` - Docker container integration and auto-discovery
   - `autocert/` - SSL certificate management with Let's Encrypt
   - `acl/` - Access control lists (IP/CIDR, country, timezone)
   - `api/` - REST API server with Swagger documentation
   - `homepage/` - WebUI dashboard and configuration interface
   - `metrics/` - System metrics and uptime monitoring
   - `idlewatcher/` - Container idle detection and power management
   - `watcher/` - File and container state watchers

3. **Sub-projects**
   - `agent/` - System agent for monitoring containers
   - `socket-proxy/` - Docker socket proxy

### Key Patterns

- **Task Management**: Use `internal/task/task.go` for managing object lifetimes and background operations
- **Concurrent Maps**: Use `github.com/puzpuzpuz/xsync/v4` instead of maps with mutexes
- **Error Handling**: Use `pkg/gperr` for nested errors with subjects
- **Configuration**: YAML-based configuration in `config/` directory

## Go Guidelines (from .cursor/rules/go.mdc)

1. Use builtin `min` and `max` functions instead of custom helpers
2. Prefer range-over-integer syntax (`for i := range 10`) over traditional loops
3. Use `xsync/v4` for concurrent maps instead of map+mutex
4. Beware of variable shadowing when making edits
5. Use `internal/task/task.go` for lifetime management:
   - `task.RootTask()` for background operations
   - `parent.Subtask()` for nested tasks
   - `OnFinished()` and `OnCancel()` callbacks for cleanup
6. Use `pkg/gperr` for complex error scenarios:
   - `gperr.Multiline()` for multiple operation attempts
   - `gperr.NewBuilder()` for collecting multiple errors
   - `gperr.NewGroup() + group.Go()` for collecting errors of multiple concurrent operations
   - `gperr.New().Subject()` for errors with subjects

## Configuration Structure

- `config/config.yml` - Main configuration
- `config/middlewares/` - HTTP middleware configurations
- `config/*.yml` - Provider and service configurations
- `.env` - Environment variables for ports and settings

## Testing

Tests are located in `internal/` packages and can be run with:

- `make test` - Run all internal package tests
- Individual package tests: `go test ./internal/config/...`
- You MUST use `-ldflags="-checklinkname=0"` otherwise compiler will complain
- prefer `testify/require` over `expect`

## Docker Integration

GoDoxy integrates with Docker by:

1. Listing all containers and reading their labels
2. Creating routes based on `proxy.aliases` label or container name
3. Watching for container/config changes and updating automatically
4. Supporting both Docker and Podman runtimes
