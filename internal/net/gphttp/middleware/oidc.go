package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/auth"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/route/routes"
	gperr "github.com/yusing/goutils/errs"
	httpevents "github.com/yusing/goutils/events/http"
	"github.com/yusing/goutils/http/httpheaders"
	strutils "github.com/yusing/goutils/strings"
)

type oidcMiddleware struct {
	IssuerURL     string            `json:"issuer_url"`
	AllowedUsers  []string          `json:"allowed_users"`
	AllowedGroups []string          `json:"allowed_groups"`
	ClientID      strutils.Redacted `json:"client_id"`
	ClientSecret  strutils.Redacted `json:"client_secret"`
	Scopes        []string          `json:"scopes"`

	authHash string
	auth     *auth.OIDCProvider

	isInitialized int32
	initMu        sync.Mutex
}

var OIDC = NewMiddleware[oidcMiddleware]()

func isOIDCAuthPath(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, auth.OIDCAuthBasePath)
}

func (amw *oidcMiddleware) finalize() error {
	errs := gperr.NewBuilder("oidc middleware error")
	if amw.IssuerURL == "" {
		amw.IssuerURL = common.OIDCIssuerURL
	}
	if amw.IssuerURL == "" {
		errs.Adds("oidc: middleware is used with no issuer provided")
	}
	if len(amw.AllowedUsers) == 0 {
		amw.AllowedUsers = common.OIDCAllowedUsers
	}
	if len(amw.AllowedGroups) == 0 {
		amw.AllowedGroups = common.OIDCAllowedGroups
	}
	if len(amw.AllowedUsers) == 0 && len(amw.AllowedGroups) == 0 {
		errs.Adds("oidc: middleware is used with no user or group allowed")
	}
	if amw.ClientID == "" {
		amw.ClientID = strutils.Redacted(common.OIDCClientID)
	}
	if amw.ClientID == "" {
		errs.Adds("oidc: middleware requires client_id")
	}
	if amw.ClientSecret == "" {
		amw.ClientSecret = strutils.Redacted(common.OIDCClientSecret)
	}
	if amw.ClientSecret == "" {
		errs.Adds("oidc: middleware requires client_secret")
	}
	if len(amw.Scopes) == 0 {
		amw.Scopes = common.OIDCScopes
	}
	if len(amw.Scopes) == 0 {
		errs.Adds("oidc: middleware requires scopes")
	}
	amw.authHash = auth.OIDCProviderHash(amw.IssuerURL, amw.ClientID.String())
	return errs.Error()
}

func (amw *oidcMiddleware) init(ctx context.Context) error {
	if atomic.LoadInt32(&amw.isInitialized) == 1 {
		return nil
	}

	return amw.initSlow(ctx)
}

func (amw *oidcMiddleware) initSlow(ctx context.Context) error {
	amw.initMu.Lock()
	if atomic.LoadInt32(&amw.isInitialized) == 1 {
		amw.initMu.Unlock()
		return nil
	}
	defer amw.initMu.Unlock()

	// If no custom credentials, authProvider remains the global one
	authProvider, err := auth.NewOIDCProvider(
		ctx,
		amw.IssuerURL,
		amw.ClientID.String(),
		amw.ClientSecret.String(),
		amw.Scopes,
		amw.AllowedUsers,
		amw.AllowedGroups,
	)
	if err != nil {
		return err
	}

	// Always trigger login on unknown paths.
	// This prevents falling back to the default login page, which applies bypass rules.
	// Without this, redirecting to the global login page could circumvent the intended route restrictions.
	authProvider.SetOnUnknownPathHandler(authProvider.LoginHandler)

	amw.auth = authProvider
	atomic.StoreInt32(&amw.isInitialized, 1)
	return nil
}

func (amw *oidcMiddleware) before(w http.ResponseWriter, r *http.Request) (proceed bool) {
	if err := amw.init(r.Context()); err != nil {
		if amw.authHash != auth.GlobalOIDCProviderHash() {
			event := log.Err(err)
			if route := routes.TryGetRoute(r); route != nil {
				event.Str("route", route.Name())
			}
			event.Str("issuer_url", amw.IssuerURL)
			event.Msg("failed to initialize oidc middleware")
		} else {
			// do not log here, global OIDC failure is logged in main()
		}

		code := http.StatusInternalServerError
		http.Error(w, http.StatusText(code), code)
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
