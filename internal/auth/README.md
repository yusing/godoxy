# internal/auth

Authentication providers supporting OIDC and username/password authentication with JWT-based sessions.

## Overview

The auth package implements authentication middleware and login handlers that integrate with GoDoxy's HTTP routing system. It provides flexible authentication that can be enabled/disabled based on configuration and supports multiple authentication providers.

### Primary consumers

- `internal/route/rules` - Authentication middleware for routes
- `internal/api/v1/auth` - Login and session management endpoints
- `internal/homepage` - WebUI login page

### Non-goals

- ACL or authorization (see `internal/acl`)
- User management database
- Multi-factor authentication
- Rate limiting (basic OIDC rate limiting only)

### Stability

Stable internal package. Public API consists of the `Provider` interface and initialization functions.

## Public API

### Exported types

```go
type LoginResult uint8

const (
    LoginResponseHandled LoginResult = iota
    LoginSessionRefreshed
)

type Provider interface {
    CheckToken(r *http.Request) error
    LoginHandler(w http.ResponseWriter, r *http.Request) LoginResult
    PostAuthCallbackHandler(w http.ResponseWriter, r *http.Request)
    LogoutHandler(w http.ResponseWriter, r *http.Request)
}
```

`LoginSessionRefreshed` means the provider verified the refreshed identity and authorization, set fresh authentication cookies, but did not commit a status, body, or redirect. The caller must complete the response according to the request boundary it owns. All `LoginResponseHandled` branches have already written their complete response.

### OIDC Provider

Each OIDC login stores its initiating local `GET` path and query in a short-lived, signed, HTTP-only transaction cookie selected by the OAuth state. The callback validates and consumes only that transaction before redirecting, so overlapping tabs remain independent. Signed malformed, absolute, and network-path targets fall back to `/`; missing, expired, or tampered transactions fail state validation.

```go
type OIDCProvider struct {
    hash          string // sha256(issuer_url\nclient_id)
    issuerURL     string
    oauthConfig   *oauth2.Config
    oidcProvider  *oidc.Provider
    oidcVerifier  *oidc.IDTokenVerifier
    endSessionURL *url.URL
    allowedUsers  []string
    allowedGroups []string
    rateLimit     *rate.Limiter
}
```

### Username/Password Provider

```go
type UserPassAuth struct {
    username string
    pwdHash  []byte
    secret   []byte
    tokenTTL time.Duration
}
```

### Exported functions

```go
func Initialize() error
```

Sets up authentication providers based on environment configuration. Returns an error when username/password authentication is selected without both `API_USER` and `API_PASSWORD`, or when an OIDC issuer is configured but cannot be reached.

```go
func IsEnabled() bool
```

Returns whether authentication is enabled. Checks `DEBUG_DISABLE_AUTH`, `API_JWT_SECRET`, and `OIDC_ISSUER_URL`.

```go
func IsOIDCEnabled() bool
```

Returns whether OIDC authentication is configured.

```go
func GetDefaultAuth() Provider
```

Returns the configured authentication provider.

```go
func AuthCheckHandler(w http.ResponseWriter, r *http.Request)
```

HTTP handler that checks if the request has a valid token. Returns 200 if valid, invokes login handler otherwise.

When an OIDC session is refreshed, it returns 200 with the fresh cookies instead of applying browser-navigation behavior. When no session can be refreshed, it returns 401 without starting an interactive login redirect, allowing API and WebSocket clients to choose their own recovery behavior.

```go
func AuthOrProceed(w http.ResponseWriter, r *http.Request) bool
```

Authenticates request or proceeds if valid. Returns `true` only when the incoming token is already valid. After an OIDC refresh it returns `false` and completes the current response: eligible HTML `GET` requests receive a local redirect, while unsafe methods, non-HTML requests, and WebSocket upgrades are rejected without replay.

```go
func NewUserPassAuth(username, password string, secret []byte, tokenTTL time.Duration) (*UserPassAuth, error)
```

Creates a new username/password auth provider with bcrypt password hashing.

```go
func NewUserPassAuthFromEnv() (*UserPassAuth, error)
```

Creates username/password auth from environment variables `API_USER`, `API_PASSWORD`, `API_JWT_SECRET`.

```go
func NewOIDCProvider(ctx context.Context, issuerURL, clientID, clientSecret string, scopes, allowedUsers, allowedGroups []string) (*OIDCProvider, error)
```

Creates a new OIDC provider. When a global provider is already initialized for the same issuer and client ID, discovery and verifier state are reused while the supplied secret, scopes, and allow lists stay on the new provider. Returns an error if the issuer cannot be reached or no allowed users/groups are configured. Route middleware can call this without enabling global OIDC.

```go
func NewOIDCProviderFromEnv(ctx context.Context) (*OIDCProvider, error)
```

Creates OIDC provider from environment variables `OIDC_ISSUER_URL`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, `OIDC_SCOPES`, etc.

## Architecture

### Core components

```mermaid
graph TD
    A[HTTP Request] --> B{Auth Enabled?}
    B -->|No| C[Proceed Direct]
    B -->|Yes| D[Check Token]
    D -->|Valid| E[Proceed]
    D -->|Invalid| F[Login Handler]

    G[OIDC Provider] --> H[Token Validation]
    I[UserPass Provider] --> J[Credential Check]

    F --> K{OIDC Configured?}
    K -->|Yes| G
    K -->|No| I

    subgraph Cookie Management
        L[Token Cookie]
        M[State Cookie]
        N[Session Cookie]
    end
```

### OIDC authentication flow

```mermaid
sequenceDiagram
    participant User
    participant App
    participant IdP

    User->>App: Access Protected Resource
    App->>App: Check Token
    alt No valid token
        App-->>User: Redirect to /auth/
        User->>IdP: Login & Authorize
        IdP-->>User: Redirect with Code
        User->>App: /auth/callback?code=...
        App->>IdP: Exchange Code for Token
        IdP-->>App: Access Token + ID Token
        App->>App: Validate Token
        App->>App: Check allowed users/groups
        App-->>User: Protected Resource
    else Valid token exists
        App-->>User: Protected Resource
    end
```

### Username/password flow

```mermaid
sequenceDiagram
    participant User
    participant App

    User->>App: POST /auth/callback
    App->>App: Validate credentials
    alt Valid
        App->>App: Generate JWT
        App-->>User: Set token cookie, redirect to /
    else Invalid
        App-->>User: 401 Unauthorized
    end
```

## Configuration Surface

### Environment variables

| Variable                 | Description                                                 |
| ------------------------ | ----------------------------------------------------------- |
| `DEBUG_DISABLE_AUTH`     | Set to "true" to disable auth for debugging                 |
| `API_JWT_SECRET`         | Secret key for JWT token validation (enables userpass auth) |
| `API_USER`               | Required username for userpass authentication               |
| `API_PASSWORD`           | Required password for userpass authentication               |
| `API_JWT_TOKEN_TTL`      | Token TTL duration (default: 24h)                           |
| `OIDC_ISSUER_URL`        | OIDC provider URL (enables OIDC)                            |
| `OIDC_CLIENT_ID`         | OIDC client ID                                              |
| `OIDC_CLIENT_SECRET`     | OIDC client secret                                          |
| `OIDC_REDIRECT_URL`      | OIDC redirect URL                                           |
| `OIDC_ALLOWED_USERS`     | Comma-separated list of allowed users                       |
| `OIDC_ALLOWED_GROUPS`    | Comma-separated list of allowed groups                      |
| `OIDC_SCOPES`            | Comma-separated OIDC scopes (default: openid, profile, email, groups) |
| `OIDC_RATE_LIMIT`        | Rate limit requests (default: 10)                           |
| `OIDC_RATE_LIMIT_PERIOD` | Rate limit period (default: 1s)                             |

### Hot-reloading

Authentication configuration requires restart. No dynamic reconfiguration is supported.

When authentication is enabled and `OIDC_ISSUER_URL` is empty, startup requires both `API_USER` and `API_PASSWORD`. Neither credential has a default. OIDC-only setups and setups with `DEBUG_DISABLE_AUTH=true` do not require these fields.

## Dependency and Integration Map

### Internal dependencies

- `internal/common` - Environment variable access

### External dependencies

- `golang.org/x/crypto/bcrypt` - Password hashing
- `github.com/coreos/go-oidc/v3/oidc` - OIDC protocol
- `golang.org/x/oauth2` - OAuth2/OIDC implementation
- `github.com/golang-jwt/jwt/v5` - JWT token handling
- `golang.org/x/time/rate` - OIDC rate limiting

### Integration points

```go
// Route middleware uses AuthOrProceed
routeHandler := func(w http.ResponseWriter, r *http.Request) {
    if !auth.AuthOrProceed(w, r) {
        return // Auth failed, login handler was invoked
    }
    // Continue with authenticated request
}
```

## Observability

### Logs

- OIDC provider initialization errors
- Token validation failures
- Rate limit exceeded events

### Metrics

No metrics are currently exposed.

## Security Considerations

- JWT tokens use HS512 signing for userpass auth
- OIDC tokens are validated against the issuer
- Session tokens are scoped by client ID to prevent conflicts
- Passwords are hashed with bcrypt (cost 10)
- OIDC rate limiting prevents brute-force attacks
- State parameter prevents CSRF attacks
- Refresh tokens are stored and invalidated on logout

## Failure Modes and Recovery

| Failure                  | Behavior                       | Recovery                      |
| ------------------------ | ------------------------------ | ----------------------------- |
| OIDC issuer unreachable  | Initialize returns error       | Fix network/URL configuration |
| Invalid JWT secret       | Initialize uses API_JWT_SECRET | Provide correct secret        |
| Token expired            | Refreshes an eligible OIDC session or starts login | Retry through the response selected by the owning request boundary |
| User not in allowed list | Returns ErrUserNotAllowed      | Add user to allowed list      |
| Rate limit exceeded      | Returns 429 Too Many Requests  | Wait for rate limit reset     |

## Usage Examples

### Basic setup

```go
// Initialize authentication during startup
err := auth.Initialize()
if err != nil {
    log.Fatal(err)
}

// Check if auth is enabled
if auth.IsEnabled() {
    log.Println("Authentication is enabled")
}

// Check OIDC status
if auth.IsOIDCEnabled() {
    log.Println("OIDC authentication configured")
}
```

### Using AuthOrProceed middleware

```go
func protectedHandler(w http.ResponseWriter, r *http.Request) {
    if !auth.AuthOrProceed(w, r) {
        return // Auth failed, login handler was invoked
    }
    // Continue with authenticated request
}
```

### Using AuthCheckHandler

```go
http.HandleFunc("/api/", auth.AuthCheckHandler(apiHandler))
```

### Custom OIDC provider

```go
provider, err := auth.NewOIDCProvider(
    ctx,
    "https://your-idp.com",
    "your-client-id",
    "your-client-secret",
    []string{"openid", "profile", "email", "groups"},
    []string{"user1", "user2"},
    []string{"group1"},
)
if err != nil {
    log.Fatal(err)
}
```

### Custom userpass provider

```go
provider, err := auth.NewUserPassAuth(
    "admin",
    "password123",
    []byte("jwt-secret-key"),
    24*time.Hour,
)
if err != nil {
    log.Fatal(err)
}
```
