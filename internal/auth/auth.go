package auth

import (
	"context"
	"net/http"

	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/net/gphttp"
)

var defaultAuth Provider

// Initialize sets up authentication providers.
func Initialize() error {
	if !IsEnabled() {
		return nil
	}

	var err error
	// Initialize OIDC if configured.
	if common.OIDCIssuerURL != "" {
		defaultAuth, err = NewOIDCProviderFromEnv()
	} else {
		defaultAuth, err = NewUserPassAuthFromEnv()
	}

	return err
}

func GetDefaultAuth() Provider {
	return defaultAuth
}

func IsEnabled() bool {
	return !common.DebugDisableAuth && (common.APIJWTSecret != nil || IsOIDCEnabled())
}

func IsOIDCEnabled() bool {
	return common.OIDCIssuerURL != ""
}

type nextHandler struct{}

var nextHandlerContextKey = nextHandler{}

func RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	if !IsEnabled() {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if err := defaultAuth.CheckToken(r); err != nil {
			if IsFrontend(r) {
				r = r.WithContext(context.WithValue(r.Context(), nextHandlerContextKey, next))
				defaultAuth.LoginHandler(w, r)
			} else {
				gphttp.ClientError(w, err, http.StatusUnauthorized)
			}
			return
		}
		next(w, r)
	}
}

func ProceedNext(w http.ResponseWriter, r *http.Request) {
	next, ok := r.Context().Value(nextHandlerContextKey).(http.HandlerFunc)
	if ok {
		next(w, r)
	} else {
		w.WriteHeader(http.StatusOK)
	}
}

func AuthCheckHandler(w http.ResponseWriter, r *http.Request) {
	if err := defaultAuth.CheckToken(r); err != nil {
		defaultAuth.LoginHandler(w, r)
	} else {
		w.WriteHeader(http.StatusOK)
	}
}
