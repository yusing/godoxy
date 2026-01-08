# agent/pkg/env

Environment configuration package for the GoDoxy Agent.

## Overview

This package manages environment variable parsing and provides a centralized location for all agent configuration options. It is automatically initialized on import.

## Variables

| Variable                   | Type             | Default                | Description                             |
| -------------------------- | ---------------- | ---------------------- | --------------------------------------- |
| `DockerSocket`             | string           | `/var/run/docker.sock` | Path to Docker socket                   |
| `AgentName`                | string           | System hostname        | Agent identifier                        |
| `AgentPort`                | int              | `8890`                 | Agent server port                       |
| `AgentSkipClientCertCheck` | bool             | `false`                | Skip mTLS certificate verification      |
| `AgentCACert`              | string           | (empty)                | Base64 Encoded CA certificate + key     |
| `AgentSSLCert`             | string           | (empty)                | Base64 Encoded server certificate + key |
| `Runtime`                  | ContainerRuntime | `docker`               | Container runtime (docker or podman)    |

## ContainerRuntime Type

```go
type ContainerRuntime string

const (
    ContainerRuntimeDocker  ContainerRuntime = "docker"
    ContainerRuntimePodman  ContainerRuntime = "podman"
)
```

## Public Functions

### DefaultAgentName

```go
func DefaultAgentName() string
```

Returns the system hostname as the default agent name. Falls back to `"agent"` if hostname cannot be determined.

### Load

```go
func Load()
```

Reloads all environment variables from the environment. Called automatically on package init, but can be called again to refresh configuration.

## Validation

The `Load()` function validates that `Runtime` is either `docker` or `podman`. An invalid runtime causes a fatal error.
