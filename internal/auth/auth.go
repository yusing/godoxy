package auth

import (
	"net/http"

	"github.com/yusing/godoxy/internal/common"
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

func ProceedNext(w http.ResponseWriter, r *http.Request) {
	next, ok := r.Context().Value(nextHandlerContextKey).(http.HandlerFunc)
	if ok {
		next(w, r)
	} else {
		w.WriteHeader(http.StatusOK)
	}
}

func AuthCheckHandler(w http.ResponseWriter, r *http.Request) {
	err := defaultAuth.CheckToken(r)
	if err != nil {
		defaultAuth.LoginHandler(w, r)
	} else {
		w.WriteHeader(http.StatusOK)
	}
}
