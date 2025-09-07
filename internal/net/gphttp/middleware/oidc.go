package middleware

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/yusing/go-proxy/internal/auth"
	"github.com/yusing/go-proxy/internal/gperr"
)

type oidcMiddleware struct {
	AllowedUsers  []string `json:"allowed_users"`
	AllowedGroups []string `json:"allowed_groups"`
	ClientID      string   `json:"client_id"`
	ClientSecret  string   `json:"client_secret"`
	Scopes        string   `json:"scopes"`

	auth *auth.OIDCProvider

	isInitialized int32
	initMu        sync.Mutex
}

var OIDC = NewMiddleware[oidcMiddleware]()

func (amw *oidcMiddleware) finalize() error {
	if !auth.IsOIDCEnabled() {
		return gperr.New("OIDC not enabled but OIDC middleware is used")
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

	// Always start with the global OIDC provider (for issuer discovery)
	authProvider, err := auth.NewOIDCProviderFromEnv()
	if err != nil {
		return err
	}

	// Check if custom client credentials are provided
	if amw.ClientID != "" && amw.ClientSecret != "" {
		// Use custom client credentials
		customProvider, err := auth.NewOIDCProviderWithCustomClient(
			authProvider,
			amw.ClientID,
			amw.ClientSecret,
		)
		if err != nil {
			return err
		}
		authProvider = customProvider
	}
	// If no custom credentials, authProvider remains the global one

	// Apply per-route user/group restrictions (these always override global)
	if len(amw.AllowedUsers) > 0 {
		authProvider.SetAllowedUsers(amw.AllowedUsers)
	}
	if len(amw.AllowedGroups) > 0 {
		authProvider.SetAllowedGroups(amw.AllowedGroups)
	}

	// Apply custom scopes if provided
	if amw.Scopes != "" {
		authProvider.SetScopes(strings.Split(amw.Scopes, ","))
	}

	amw.auth = authProvider
	return nil
}

func (amw *oidcMiddleware) before(w http.ResponseWriter, r *http.Request) (proceed bool) {
	if err := amw.init(); err != nil {
		// no need to log here, main OIDC should've already failed and logged
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return false
	}

	if r.URL.Path == auth.OIDCLogoutPath {
		amw.auth.LogoutHandler(w, r)
		return true
	}

	err := amw.auth.CheckToken(r)
	if err == nil {
		return true
	}

	switch {
	case errors.Is(err, auth.ErrMissingOAuthToken):
		amw.auth.HandleAuth(w, r)
	default:
		auth.WriteBlockPage(w, http.StatusForbidden, err.Error(), auth.OIDCLogoutPath)
	}
	return false
}
