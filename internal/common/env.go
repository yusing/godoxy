package common

import (
	"os"
	"strings"
	"time"

	"github.com/yusing/goutils/env"
)

func init() {
	env.SetPrefixes("GODOXY_", "GOPROXY_", "")
}

var (
	IsTest  = env.GetEnvBool("TEST", false) || strings.HasSuffix(os.Args[0], ".test")
	IsDebug = env.GetEnvBool("DEBUG", IsTest)
	IsTrace = env.GetEnvBool("TRACE", false) && IsDebug

	HTTP3Enabled = env.GetEnvBool("HTTP3_ENABLED", true)

	ProxyHTTPAddr,
	ProxyHTTPHost,
	ProxyHTTPPort,
	ProxyHTTPURL = env.GetAddrEnv("HTTP_ADDR", ":80", "http")

	ProxyHTTPSAddr,
	ProxyHTTPSHost,
	ProxyHTTPSPort,
	ProxyHTTPSURL = env.GetAddrEnv("HTTPS_ADDR", ":443", "https")

	APIHTTPAddr,
	APIHTTPHost,
	APIHTTPPort,
	APIHTTPURL = env.GetAddrEnv("API_ADDR", "127.0.0.1:8888", "http")

	APIJWTSecure   = env.GetEnvBool("API_JWT_SECURE", true)
	APIJWTSecret   = decodeJWTKey(env.GetEnvString("API_JWT_SECRET", ""))
	APIJWTTokenTTL = env.GetEnvDuation("API_JWT_TOKEN_TTL", 24*time.Hour)
	APIUser        = env.GetEnvString("API_USER", "admin")
	APIPassword    = env.GetEnvString("API_PASSWORD", "password")

	APISkipOriginCheck = env.GetEnvBool("API_SKIP_ORIGIN_CHECK", false) // skip this in UI Demo

	DebugDisableAuth = env.GetEnvBool("DEBUG_DISABLE_AUTH", false)

	// OIDC Configuration.
	OIDCIssuerURL     = env.GetEnvString("OIDC_ISSUER_URL", "")
	OIDCClientID      = env.GetEnvString("OIDC_CLIENT_ID", "")
	OIDCClientSecret  = env.GetEnvString("OIDC_CLIENT_SECRET", "")
	OIDCScopes        = env.GetEnvCommaSep("OIDC_SCOPES", "openid, profile, email, groups")
	OIDCAllowedUsers  = env.GetEnvCommaSep("OIDC_ALLOWED_USERS", "")
	OIDCAllowedGroups = env.GetEnvCommaSep("OIDC_ALLOWED_GROUPS", "")

	// metrics configuration
	MetricsDisableCPU     = env.GetEnvBool("METRICS_DISABLE_CPU", false)
	MetricsDisableMemory  = env.GetEnvBool("METRICS_DISABLE_MEMORY", false)
	MetricsDisableDisk    = env.GetEnvBool("METRICS_DISABLE_DISK", false)
	MetricsDisableNetwork = env.GetEnvBool("METRICS_DISABLE_NETWORK", false)
	MetricsDisableSensors = env.GetEnvBool("METRICS_DISABLE_SENSORS", false)

	ForceResolveCountry = env.GetEnvBool("FORCE_RESOLVE_COUNTRY", false)
)
