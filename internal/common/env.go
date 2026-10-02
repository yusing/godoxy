package common

import (
	env "github.com/yusing/godoxy/internal/env"
)

var (
	IsTest  = env.Bool("TEST")
	IsDebug = env.Bool("DEBUG")
	IsTrace = env.Bool("TRACE")

	InitTimeout = env.Duration("INIT_TIMEOUT")

	ShortLinkPrefix = env.String("SHORTLINK_PREFIX")

	ProxyHTTPAddr,
	ProxyHTTPHost,
	ProxyHTTPPort,
	ProxyHTTPURL = env.Address("HTTP_ADDR", "http")

	ProxyHTTPSAddr,
	ProxyHTTPSHost,
	ProxyHTTPSPort,
	ProxyHTTPSURL = env.Address("HTTPS_ADDR", "https")

	APIHTTPAddr,
	APIHTTPHost,
	APIHTTPPort,
	APIHTTPURL = env.Address("API_ADDR", "http")

	LocalAPIHTTPAddr,
	LocalAPIHTTPHost,
	LocalAPIHTTPPort,
	LocalAPIHTTPURL = env.Address("LOCAL_API_ADDR", "http")
	LocalAPIAllowNonLoopback = env.Bool("LOCAL_API_ALLOW_NON_LOOPBACK")

	APIJWTSecure   = env.Bool("API_JWT_SECURE")
	APIJWTSecret   = decodeJWTKey(env.String("API_JWT_SECRET"))
	APIJWTTokenTTL = env.Duration("API_JWT_TOKEN_TTL")
	APIUser        = env.String("API_USER")
	APIPassword    = env.String("API_PASSWORD")

	APISkipOriginCheck = env.Bool("API_SKIP_ORIGIN_CHECK") // skip this in UI Demo

	DebugDisableAuth = env.Bool("DEBUG_DISABLE_AUTH")

	// OIDC Configuration.
	OIDCIssuerURL       = env.String("OIDC_ISSUER_URL")
	OIDCClientID        = env.String("OIDC_CLIENT_ID")
	OIDCClientSecret    = env.String("OIDC_CLIENT_SECRET")
	OIDCScopes          = env.CommaSep("OIDC_SCOPES")
	OIDCAllowedUsers    = env.CommaSep("OIDC_ALLOWED_USERS")
	OIDCAllowedGroups   = env.CommaSep("OIDC_ALLOWED_GROUPS")
	OIDCRateLimit       = env.Int("OIDC_RATE_LIMIT")
	OIDCRateLimitPeriod = env.Duration("OIDC_RATE_LIMIT_PERIOD")

	FrontendAliasesLegacy = env.CommaSep("FRONTEND_ALIASES")

	// metrics configuration
	MetricsDisableCPU     = env.Bool("METRICS_DISABLE_CPU")
	MetricsDisableMemory  = env.Bool("METRICS_DISABLE_MEMORY")
	MetricsDisableDisk    = env.Bool("METRICS_DISABLE_DISK")
	MetricsDisableNetwork = env.Bool("METRICS_DISABLE_NETWORK")
	MetricsDisableSensors = env.Bool("METRICS_DISABLE_SENSORS")

	MaxMindCountryOnly = env.Bool("MAXMIND_COUNTRY_ONLY")

	ForceResolveCountry = env.Bool("FORCE_RESOLVE_COUNTRY")

	SNIRoutingForTCPRoutes = env.Bool("SNI_ROUTING_FOR_TCP_ROUTES")
)
