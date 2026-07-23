package auth

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"

	"github.com/yusing/godoxy/internal/common"
)

type providerHolder struct {
	provider Provider
}

var defaultAuth atomic.Pointer[providerHolder]

var errMissingUserPassCredentials = errors.New(
	"GODOXY_API_USER and GODOXY_API_PASSWORD must be set when authentication is enabled without OIDC",
)

// Initialize sets up authentication providers.
func Initialize(ctx context.Context) error {
	// Validate before IsEnabled: omitting the JWT secret is not an explicit
	// request to disable authentication.
	if err := validateUserPassCredentials(); err != nil {
		return err
	}
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if !IsEnabled() {
		setDefaultAuth(nil)
		return nil
	}

	var (
		provider Provider
		err      error
	)
	// Initialize OIDC if configured.
	if common.OIDCIssuerURL != "" {
		provider, err = NewOIDCProviderFromEnv(ctx)
	} else {
		provider, err = NewUserPassAuthFromEnv()
	}
	if err != nil {
		return err
	}
	if err := context.Cause(ctx); err != nil {
		return err
	}
	setDefaultAuth(provider)
	return nil
}

func validateUserPassCredentials() error {
	if common.DebugDisableAuth || IsOIDCEnabled() {
		return nil
	}
	if common.APIUser == "" || common.APIPassword == "" {
		return errMissingUserPassCredentials
	}
	return nil
}

func GetDefaultAuth() Provider {
	holder := defaultAuth.Load()
	if holder == nil {
		return nil
	}
	return holder.provider
}

func setDefaultAuth(provider Provider) {
	if provider == nil {
		defaultAuth.Store(nil)
		return
	}
	defaultAuth.Store(&providerHolder{provider: provider})
}

func IsEnabled() bool {
	return !common.DebugDisableAuth && (common.APIJWTSecret != nil || IsOIDCEnabled())
}

func IsOIDCEnabled() bool {
	return common.OIDCIssuerURL != ""
}

type nextHandler struct{}

var nextHandlerContextKey = nextHandler{}

func ProceedNext(w http.ResponseWriter, r *http.Request) {
	next, ok := r.Context().Value(nextHandlerContextKey).(http.HandlerFunc)
	if ok {
		next(w, r)
	} else {
		w.WriteHeader(http.StatusOK)
	}
}

func AuthCheckHandler(w http.ResponseWriter, r *http.Request) {
	provider := GetDefaultAuth()
	if provider == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	err := provider.CheckToken(r)
	if err != nil {
		provider.LoginHandler(w, r)
	} else {
		w.WriteHeader(http.StatusOK)
	}
}

func AuthOrProceed(w http.ResponseWriter, r *http.Request) (proceed bool) {
	provider := GetDefaultAuth()
	if provider == nil {
		if IsEnabled() {
			http.Error(w, "authentication is initializing", http.StatusServiceUnavailable)
			return false
		}
		return true
	}
	err := provider.CheckToken(r)
	if err != nil {
		provider.LoginHandler(w, r)
		return false
	}
	return true
}
