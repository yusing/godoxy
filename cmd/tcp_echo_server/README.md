# cmd/tcp_echo_server

Minimal TCP echo service for development testing of SNI TCP routing.

## Listeners

- `:9000` — plaintext TCP echo.
- `:9443` — TLS echo using an ephemeral self-signed ECDSA certificate generated at process start.

## Dev compose routes

`dev.compose.yml` wires this service with two aliases for issue #230 testing:

- `tcp-echo-passthrough` routes `443:9443` with `scheme: tcp` for TLS passthrough (for example, SNI `tcp-echo-passthrough.my.app` when `.my.app` is a match domain).
- `tcp-echo-termination` routes `443:9000` with `scheme: tcp` and `tls_termination: true` for TLS termination before plaintext upstream echo (for example, SNI `tcp-echo-termination.my.app` when `.my.app` is a match domain).

Health checks are disabled because this service speaks raw TCP, not HTTP.

## Make test recipe

`make tcp-echo-test` starts the `tcp_echo_server` service from `dev.compose.yml`, then connects to `localhost:443` with SNI for both Docker-label routes. This tests GoDoxy TLS passthrough and TLS termination routing, not direct fixture ports.
