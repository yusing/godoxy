package auth

import (
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

func RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	if IsEnabled() {
		return func(w http.ResponseWriter, r *http.Request) {
			if err := defaultAuth.CheckToken(r); err != nil {
				gphttp.ClientError(w, err, http.StatusUnauthorized)
			} else {
				next(w, r)
			}
		}
	}
	return next
}
