// Package env defines the GoDoxy server environment contract shared by runtime consumers and documentation.
package env

import (
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Definition describes one server setting. Default is the normal, non-test default.
// Sensitive settings are never included in diagnostic values.
type Definition struct {
	Name        string
	Type        string
	Default     string
	Description string
	Sensitive   bool
	Development bool
	// Compose settings use exact names and are not read as server options.
	Compose bool
	// Required settings are active in the generated example.
	Required bool
	// Example overrides Default only for deployment-specific required inputs.
	Example string
}

var definitions = []Definition{
	{Name: "TAG", Type: "string", Default: "latest", Description: "Container image tag.", Compose: true, Required: true},
	{Name: "TZ", Type: "string", Default: "Etc/UTC", Description: "IANA timezone for log timestamps.", Compose: true, Required: true},
	{Name: "GODOXY_UID", Type: "int", Default: "1000", Description: "Container user ID; must own mounted directories.", Compose: true, Required: true},
	{Name: "GODOXY_GID", Type: "int", Default: "1000", Description: "Container group ID; must own mounted directories.", Compose: true, Required: true},
	{Name: "DOCKER_SOCKET", Type: "string", Default: "/var/run/docker.sock", Description: "Host Docker socket path; use /var/run/podman/podman.sock for Podman.", Compose: true, Required: true},
	{Name: "LISTEN_ADDR", Type: "address", Default: "127.0.0.1:2375", Description: "Socket-proxy listener used by the Compose example.", Compose: true, Required: true},
	{Name: "HTTP_ADDR", Type: "address", Default: ":80", Description: "HTTP proxy listener."},
	{Name: "HTTPS_ADDR", Type: "address", Default: ":443", Description: "HTTPS proxy listener."},
	{Name: "HTTP3_ENABLED", Type: "bool", Default: "false", Description: "Enable HTTP/3 on HTTPS, except with PROXY protocol."},
	{Name: "SNI_ROUTING_FOR_TCP_ROUTES", Type: "bool", Default: "true", Description: "Route TCP over the shared HTTPS listener using TLS SNI."},
	{Name: "API_ADDR", Type: "address", Default: "127.0.0.1:8888", Description: "API and WebUI backend listener."},
	{Name: "LOCAL_API_ADDR", Type: "address", Default: "", Description: "Unauthenticated local API listener; empty disables it."},
	{Name: "LOCAL_API_ALLOW_NON_LOOPBACK", Type: "bool", Default: "false", Description: "Allow unauthenticated local API outside loopback. Exposes unrestricted access."},
	{Name: "API_JWT_SECURE", Type: "bool", Default: "true", Description: "Require HTTPS for login cookies."},
	{Required: true, Name: "API_JWT_SECRET", Type: "string", Default: "", Description: "JWT signing key; generate with openssl rand -base64 32. Empty generates a temporary random key.", Sensitive: true},
	{Name: "API_JWT_TOKEN_TTL", Type: "duration", Default: "24h", Description: "Login token lifetime."},
	{Required: true, Name: "API_USER", Type: "string", Default: "", Description: "WebUI username; required with password unless OIDC or disabled authentication is used.", Sensitive: true},
	{Required: true, Name: "API_PASSWORD", Type: "string", Default: "", Description: "WebUI password.", Sensitive: true},
	{Name: "OIDC_ISSUER_URL", Type: "string", Default: "", Description: "OIDC issuer URL; empty disables OIDC.", Sensitive: true},
	{Name: "OIDC_CLIENT_ID", Type: "string", Default: "", Description: "OIDC client ID.", Sensitive: true},
	{Name: "OIDC_CLIENT_SECRET", Type: "string", Default: "", Description: "OIDC client secret.", Sensitive: true},
	{Name: "OIDC_SCOPES", Type: "list", Default: "openid, profile, email, groups", Description: "OIDC scopes; add offline_access if supported by the identity provider."},
	{Name: "OIDC_ALLOWED_USERS", Type: "list", Default: "", Description: "Comma-separated users allowed to log in.", Sensitive: true},
	{Name: "OIDC_ALLOWED_GROUPS", Type: "list", Default: "", Description: "Comma-separated groups allowed to log in.", Sensitive: true},
	{Name: "OIDC_RATE_LIMIT", Type: "int", Default: "10", Description: "OIDC login requests allowed per period."},
	{Name: "OIDC_RATE_LIMIT_PERIOD", Type: "duration", Default: "1s", Description: "OIDC login rate limit period."},
	{Name: "FRONTEND_ALIASES", Type: "list", Default: "godoxy", Description: "Legacy WebUI aliases, used only when webui.aliases is empty."},
	{Name: "SHORTLINK_PREFIX", Type: "string", Default: "go", Description: "Path prefix for short links."},
	{Name: "INIT_TIMEOUT", Type: "duration", Default: "1m", Description: "Startup initialization timeout; unavailable providers retry in the background."},
	{Required: true, Example: "tcp://${LISTEN_ADDR}", Name: "DOCKER_HOST", Type: "string", Default: "unix:///var/run/docker.sock", Description: "Endpoint used by providers configured as $DOCKER_HOST.", Sensitive: true},
	{Name: "METRICS_DISABLE_CPU", Type: "bool", Default: "false", Description: "Disable cpu metrics collection."},
	{Name: "METRICS_DISABLE_MEMORY", Type: "bool", Default: "false", Description: "Disable memory metrics collection."},
	{Name: "METRICS_DISABLE_DISK", Type: "bool", Default: "false", Description: "Disable disk metrics collection."},
	{Name: "METRICS_DISABLE_NETWORK", Type: "bool", Default: "false", Description: "Disable network metrics collection."},
	{Name: "METRICS_DISABLE_SENSORS", Type: "bool", Default: "false", Description: "Disable sensors metrics collection."},
	{Name: "MAXMIND_COUNTRY_ONLY", Type: "bool", Default: "false", Description: "Use Country instead of City databases; opt out of City downloads when unavailable. Country databases do not support timezone rules."},
	{Name: "FORCE_RESOLVE_COUNTRY", Type: "bool", Default: "false", Description: "Always resolve the client country with GeoIP."},
	{Name: "DEBUG", Type: "bool", Default: "false", Description: "General debug logging. Defaults to TEST or a Go test executable; explicit false overrides that default."},
	{Name: "TRACE", Type: "bool", Default: "false", Description: "Trace logging; effective only when DEBUG is enabled."},
	{Name: "SERVER_DEBUG", Type: "bool", Default: "false", Description: "HTTP server debug logging; explicit DEBUG=true also enables this, but test-derived DEBUG does not."},
	{Name: "WEBSOCKET_DEBUG", Type: "bool", Default: "false", Description: "Websocket debug logging; explicit DEBUG=true also enables this, but test-derived DEBUG does not."},
	{Name: "API_SKIP_ORIGIN_CHECK", Type: "bool", Default: "false", Description: "Skip API origin checks. Development only.", Development: true},
	{Name: "DEBUG_DISABLE_AUTH", Type: "bool", Default: "false", Description: "Disable authentication entirely. Never enable in production.", Development: true},
	{Name: "TEST", Type: "bool", Default: "false", Description: "Test mode; Go test executables always enable this. Development only.", Development: true},
}

// Definitions returns a copy in documentation order without reading the environment.
func Definitions() []Definition { return slices.Clone(definitions) }

func definition(key string) Definition {
	for _, d := range definitions {
		if d.Name == key {
			return d
		}
	}
	panic("unregistered GoDoxy environment setting: " + key)
}

func String(key string) string { return GetEnvString(key, definition(key).Default) }
func Int(key string) int {
	d, _ := strconv.Atoi(definition(key).Default)
	return GetEnvInt(key, d)
}
func Duration(key string) time.Duration {
	d, _ := time.ParseDuration(definition(key).Default)
	return GetEnvDuation(key, d)
}
func CommaSep(key string) []string { return GetEnvCommaSep(key, definition(key).Default) }
func Address(key, scheme string) (addr, host string, port int, fullURL string) {
	return GetAddrEnv(key, definition(key).Default, scheme)
}
func rawBool(key string) bool {
	d, _ := strconv.ParseBool(definition(key).Default)
	return GetEnvBool(key, d)
}
func Bool(key string) bool {
	switch key {
	case "TEST":
		return rawBool(key) || strings.HasSuffix(os.Args[0], ".test")
	case "DEBUG":
		return GetEnvBool(key, Bool("TEST"))
	case "TRACE":
		return rawBool(key) && Bool("DEBUG")
	case "SERVER_DEBUG", "WEBSOCKET_DEBUG":
		return rawBool(key) || rawBool("DEBUG")
	default:
		return rawBool(key)
	}
}
