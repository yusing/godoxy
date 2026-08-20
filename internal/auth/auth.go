package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/yusing/godoxy/internal/common"
	httputils "github.com/yusing/goutils/http"
	"github.com/yusing/goutils/http/httpheaders"
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

func globalOIDCProvider() *OIDCProvider {
	oidc, _ := GetDefaultAuth().(*OIDCProvider)
	return oidc
}

func GlobalOIDCProviderHash() string {
	if oidc := globalOIDCProvider(); oidc != nil {
		return oidc.hash
	}
	return ""
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

func AuthCheckHandler(w http.ResponseWriter, r *http.Request) {
	provider := GetDefaultAuth()
	if provider == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	err := provider.CheckToken(r)
	if err != nil {
		if provider.LoginHandler(w, r) == LoginSessionRefreshed {
			w.WriteHeader(http.StatusOK)
		}
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
		if provider.LoginHandler(w, r) == LoginSessionRefreshed {
			if r.Method == http.MethodGet &&
				httputils.GetAccept(r.Header).AcceptHTML() &&
				!httpheaders.IsWebsocket(r.Header) {
				RedirectToRequestURI(w, r)
			} else {
				http.Error(w, "authentication is required", http.StatusForbidden)
			}
		}
		return false
	}
	return true
}

// RedirectToRequestURI redirects to the request path and query only when they
// form a local origin-form target.
func RedirectToRequestURI(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, localRequestURI(r), http.StatusFound)
}

func localRequestURI(r *http.Request) string {
	path := r.URL.EscapedPath()
	if path == "" || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return "/"
	}
	if r.URL.ForceQuery || r.URL.RawQuery != "" {
		return path + "?" + r.URL.RawQuery
	}
	return path
}
