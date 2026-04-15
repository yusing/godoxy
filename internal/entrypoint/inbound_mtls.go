package entrypoint

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/types"
	gperr "github.com/yusing/goutils/errs"
)

func compileInboundMTLSProfiles(profiles map[string]types.InboundMTLSProfile) (map[string]*x509.CertPool, error) {
	if len(profiles) == 0 {
		return map[string]*x509.CertPool{}, nil
	}

	compiled := make(map[string]*x509.CertPool, len(profiles))
	errs := gperr.NewBuilder("inbound mTLS profiles error")

	for name, profile := range profiles {
		if err := profile.Validate(); err != nil {
			errs.AddSubjectf(err, "profiles.%s", name)
			continue
		}

		pool, err := buildInboundMTLSCAPool(profile)
		if err != nil {
			errs.AddSubjectf(err, "profiles.%s", name)
			continue
		}
		compiled[name] = pool
	}

	if err := errs.Error(); err != nil {
		return nil, err
	}
	return compiled, nil
}

func buildInboundMTLSCAPool(profile types.InboundMTLSProfile) (*x509.CertPool, error) {
	var pool *x509.CertPool

	if profile.UseSystemCAs {
		systemPool, err := x509.SystemCertPool()
		if err != nil {
			return nil, err
		}
		if systemPool != nil {
			pool = systemPool
		}
	}
	if pool == nil {
		pool = x509.NewCertPool()
	}

	for _, file := range profile.CAFiles {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, gperr.PrependSubject(err, file)
		}
		if !pool.AppendCertsFromPEM(data) {
			return nil, gperr.PrependSubject(errors.New("failed to parse CA certificates"), file)
		}
	}

	return pool, nil
}

func (ep *Entrypoint) SetInboundMTLSProfiles(profiles map[string]types.InboundMTLSProfile) error {
	compiled, err := compileInboundMTLSProfiles(profiles)
	if err != nil {
		return err
	}
	if profileRef := ep.cfg.InboundMTLSProfile; profileRef != "" {
		if _, ok := compiled[profileRef]; !ok {
			return fmt.Errorf("entrypoint inbound mTLS profile %q not found", profileRef)
		}
	}
	ep.inboundMTLSProfiles = compiled
	return nil
}

func (srv *httpServer) mutateServerTLSConfig(base *tls.Config) *tls.Config {
	if base == nil {
		return base
	}
	// resolveForServerName could return nil, nil
	resolveForServerName := func(serverName string) (*x509.CertPool, error) {
		return srv.resolveInboundMTLSProfileForServerName(serverName, true)
	}

	pool, err := srv.resolveInboundMTLSProfileForGlobal()
	if err != nil {
		log.Err(err).Msg("inbound mTLS: failed to resolve global profile, falling back to per-route mTLS")
		resolveForServerName = func(serverName string) (*x509.CertPool, error) {
			return srv.resolveInboundMTLSProfileForServerName(serverName, false)
		}
	}
	if pool != nil && err == nil {
		return applyInboundMTLSProfile(base, pool)
	}

	cfg := base.Clone()
	cfg.GetConfigForClient = func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		pool, err := resolveForServerName(hello.ServerName)
		if err != nil {
			return nil, err
		}
		if pool != nil {
			return applyInboundMTLSProfile(base, pool), nil
		}
		return cloneTLSConfig(base), nil
	}
	return cfg
}

func applyInboundMTLSProfile(base *tls.Config, pool *x509.CertPool) *tls.Config {
	cfg := cloneTLSConfig(base)
	cfg.ClientAuth = tls.RequireAndVerifyClientCert
	cfg.ClientCAs = pool
	return cfg
}

func cloneTLSConfig(base *tls.Config) *tls.Config {
	cfg := base.Clone()
	cfg.GetConfigForClient = nil
	return cfg
}

func ValidateInboundMTLSProfileRef(profileRef, globalProfile string, profiles map[string]types.InboundMTLSProfile) error {
	if profileRef == "" {
		return nil
	}
	if globalProfile != "" {
		return errors.New("route inbound_mtls_profile is not supported when entrypoint.inbound_mtls_profile is configured")
	}
	if _, ok := profiles[profileRef]; !ok {
		return fmt.Errorf("inbound mTLS profile %q not found", profileRef)
	}
	return nil
}

func (srv *httpServer) resolveInboundMTLSProfileForServerName(serverName string, allowGlobal bool) (*x509.CertPool, error) {
	if serverName == "" {
		return nil, errSecureRouteRequiresSNI
	}
	if len(srv.ep.inboundMTLSProfiles) == 0 {
		return nil, errMTLSNotEnabled
	}
	pool, err := srv.resolveInboundMTLSProfileForRoute(srv.FindRoute(serverName))
	if err != nil {
		if !errors.Is(err, errMTLSNotEnabled) && allowGlobal {
			return srv.resolveInboundMTLSProfileForGlobal()
		}
		return nil, err
	}
	return pool, nil
}

func (srv *httpServer) resolveInboundMTLSProfileForRoute(route types.HTTPRoute) (pool *x509.CertPool, err error) {
	if route == nil {
		return nil, errors.New("route is nil")
	}
	if ref := route.InboundMTLSProfileRef(); ref != "" {
		if pool, ok := srv.lookupInboundMTLSProfile(ref); ok {
			return pool, nil
		}
		return nil, fmt.Errorf("route %q inbound mTLS profile %q not found", route.Name(), ref)
	}
	return nil, errMTLSNotEnabled
}

func (srv *httpServer) resolveInboundMTLSProfileForGlobal() (pool *x509.CertPool, err error) {
	if globalRef := srv.ep.cfg.InboundMTLSProfile; globalRef != "" {
		if pool, ok := srv.lookupInboundMTLSProfile(globalRef); ok {
			return pool, nil
		}
		return nil, fmt.Errorf("entrypoint inbound mTLS profile %q not found", globalRef)
	}
	return nil, errMTLSNotEnabled
}

func (srv *httpServer) lookupInboundMTLSProfile(ref string) (*x509.CertPool, bool) {
	if len(srv.ep.inboundMTLSProfiles) == 0 { // nil or empty map
		return nil, false
	}
	pool, ok := srv.ep.inboundMTLSProfiles[ref]
	return pool, ok
}
