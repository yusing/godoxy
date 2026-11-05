package entrypoint

import "errors"

var (
	errSecureRouteRequiresSNI = errors.New("secure route requires matching TLS SNI")
	errSecureRouteMisdirected = errors.New("secure route host must match TLS SNI")
	errStrictSNIRequired      = errors.New("TLS SNI required")
	errStrictSNIMisdirected   = errors.New("host must match TLS SNI")
)
