package middleware

import (
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/yusing/go-proxy/internal/api/v1/auth"
	E "github.com/yusing/go-proxy/internal/error"
)

type oidcMiddleware struct {
	AllowedUsers  []string `json:"allowed_users"`
	AllowedGroups []string `json:"allowed_groups"`

	auth    auth.Provider
	authMux *http.ServeMux

	isInitialized int32
	initMu        sync.Mutex
}

var OIDC = NewMiddleware[oidcMiddleware]()

func (amw *oidcMiddleware) finalize() error {
	if !auth.IsOIDCEnabled() {
		return E.New("OIDC not enabled but OIDC middleware is used")
	}
	return nil
}

func (amw *oidcMiddleware) init() error {
	if atomic.LoadInt32(&amw.isInitialized) == 1 {
		return nil
	}

	return amw.initSlow()
}

func (amw *oidcMiddleware) initSlow() error {
	amw.initMu.Lock()
	if amw.isInitialized == 1 {
		amw.initMu.Unlock()
		return nil
	}

	defer func() {
		amw.isInitialized = 1
		amw.initMu.Unlock()
	}()

	authProvider, err := auth.NewOIDCProviderFromEnv()
	if err != nil {
		return err
	}

	authProvider.SetIsMiddleware(true)
	if len(amw.AllowedUsers) > 0 {
		authProvider.SetAllowedUsers(amw.AllowedUsers)
	}
	if len(amw.AllowedGroups) > 0 {
		authProvider.SetAllowedGroups(amw.AllowedGroups)
	}

	amw.authMux = http.NewServeMux()
	amw.authMux.HandleFunc(auth.OIDCMiddlewareCallbackPath, authProvider.LoginCallbackHandler)
	amw.authMux.HandleFunc(auth.OIDCLogoutPath, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
	amw.authMux.HandleFunc("/", authProvider.RedirectLoginPage)
	amw.auth = authProvider
	return nil
}

func (amw *oidcMiddleware) before(w http.ResponseWriter, r *http.Request) (proceed bool) {
	if err := amw.init(); err != nil {
		// no need to log here, main OIDC may already failed and logged
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return false
	}

	if err := amw.auth.CheckToken(r); err != nil {
		amw.authMux.ServeHTTP(w, r)
		return false
	}
	if r.URL.Path == auth.OIDCLogoutPath {
		amw.auth.LogoutCallbackHandler(w, r)
		return false
	}
	return true
}
