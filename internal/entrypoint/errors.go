package entrypoint

import "errors"

var (
	errSecureRouteRequiresSNI = errors.New("secure route requires matching TLS SNI")
	errSecureRouteMisdirected = errors.New("secure route host must match TLS SNI")
)
