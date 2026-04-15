package entrypoint

import "errors"

var (
	// sentinel error for when mTLS is not enabled
	errMTLSNotEnabled = errors.New("mTLS is not enabled")

	errSecureRouteRequiresSNI = errors.New("secure route requires matching TLS SNI")
	errSecureRouteMisdirected = errors.New("secure route host must match TLS SNI")
)
