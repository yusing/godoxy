package middleware

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/auth"
	httpevents "github.com/yusing/goutils/events/http"
	"github.com/yusing/goutils/http/httpheaders"
	strutils "github.com/yusing/goutils/strings"
)

type oidcMiddleware struct {
	AllowedUsers  []string          `json:"allowed_users"`
	AllowedGroups []string          `json:"allowed_groups"`
	ClientID      strutils.Redacted `json:"client_id"`
	ClientSecret  strutils.Redacted `json:"client_secret"`
	Scopes        string            `json:"scopes"`

	auth *auth.OIDCProvider

	isInitialized int32
	initMu        sync.Mutex
}

var OIDC = NewMiddleware[oidcMiddleware]()

func isOIDCAuthPath(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, auth.OIDCAuthBasePath)
}

func (amw *oidcMiddleware) finalize() error {
	if !auth.IsOIDCEnabled() {
		log.Error().Msg("OIDC not enabled but OIDC middleware is used")
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
	if atomic.LoadInt32(&amw.isInitialized) == 1 {
		amw.initMu.Unlock()
		return nil
	}
	defer amw.initMu.Unlock()

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
			amw.ClientID.String(),
			amw.ClientSecret.String(),
		)
		if err != nil {
			return err
		}
		authProvider = customProvider
	}
	// If no custom credentials, authProvider remains the global one

	// Always trigger login on unknown paths.
	// This prevents falling back to the default login page, which applies bypass rules.
	// Without this, redirecting to the global login page could circumvent the intended route restrictions.
	authProvider.SetOnUnknownPathHandler(authProvider.LoginHandler)

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
	atomic.StoreInt32(&amw.isInitialized, 1)
	return nil
}

func (amw *oidcMiddleware) before(w http.ResponseWriter, r *http.Request) (proceed bool) {
	if !auth.IsOIDCEnabled() {
		return true
	}

	if err := amw.init(); err != nil {
		// no need to log here, main OIDC should've already failed and logged
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return false
	}

	if r.URL.Path == auth.OIDCLogoutPath {
		amw.auth.LogoutHandler(w, r)
		return false
	}

	err := amw.auth.CheckToken(r)
	if err == nil {
		return true
	}

	emitBlockedEvent := func() {
		if r.Method != http.MethodHead {
			httpevents.Blocked(r, "OIDC", err.Error())
		}
	}

	isGet := r.Method == http.MethodGet
	isWS := httpheaders.IsWebsocket(r.Header)
	switch {
	case r.Method == http.MethodHead:
		w.WriteHeader(http.StatusOK)
	case !isGet, isWS:
		http.Error(w, err.Error(), http.StatusForbidden)
		reqType := r.Method
		if isWS {
			reqType = "WebSocket"
		}
		OIDC.LogWarn(r).Msgf("[OIDC] %s request blocked.\nConsider adding bypass rule for this path if needed", reqType)
		emitBlockedEvent()
		return false
	case errors.Is(err, auth.ErrMissingOAuthToken):
		amw.auth.HandleAuth(w, r)
	default:
		auth.WriteBlockPage(w, http.StatusForbidden, err.Error(), "Logout", auth.OIDCLogoutPath)
		emitBlockedEvent()
	}
	return false
}
