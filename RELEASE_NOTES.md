# GoDoxy v0.24.0 Release Notes

## New

- **Core/Agent**: stream tunneling (TLS/dTLS) with multiplexed TLS port supporting HTTP API and custom stream protocol via ALPN #188
- **Core/Agent**: TCP and DTLS/UDP stream tunneling with health-check support and runtime capability detection
- **Core/Router**: TCP/UDP route configurable bind address support
- **WebUI/Autocert**: multiple certificates display with Carousel component
- **WebUI/Agent**: stream support status display (TCP/UDP capabilities)

## Changes

- **Core/Agent**: refactored HTTP server to use direct setup instead of goutils/server helper
- **Core/HealthCheck**: restructured into dedicated `internal/health/check/` package
- **Core/Dockerfile**: updated to use bun runtime with distroless base images
- **Core/Compose**: updated agent compose template to expose TCP and UDP ports for stream tunneling

## Fixes

- **Core/Stream**: fixed stream headers with read deadlines to prevent hangs
- **Core/Icon**: fixed icons provider initialization on first load
- **Core/Docker**: fixed TLS verification and dial handling for custom Docker providers
- **Core/Stream**: fixed hostname handling for stream routes
- **Core/HTTP**: fixed HTTPS redirect for IPv6 with `net.JoinHostPort`
- **Core/Stream**: fixed remote stream scheme for IPv4 and IPv6 addresses
- **Core/HealthCheck**: fixed panic on TLS errors during HTTP health checks
- **Core/Stream**: fixed nil panic for excluded routes
- **Core/Store**: fixed empty segment handling in nested paths

## Refactoring

- **Core/Agent**: extracted agent pool and HTTP utilities to dedicated package
- **Core/Route**: improved References method for FQDN alias handling
- **Core/Icon**: reorganized with health checking and retry logic
- **Core/Error**: replaced `gperr.Builder` with `gperr.Group` for concurrent error handling
- **Core/Utils**: removed `internal/utils` entirely, moved `RefCounter` to goutils
