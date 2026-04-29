# cmd/autocert

Internal oneshot helper binary for ACME certificate lifecycle work.

- Reads a config snapshot written by the main GoDoxy process
- Performs one obtain/renew action, then exits
- Reuses `internal/autocert.Provider` for ACME logic and error handling
- Never serves public traffic directly
