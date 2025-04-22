package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/utils"
	"github.com/yusing/go-proxy/internal/utils/strutils"
	"golang.org/x/oauth2"
)

type (
	OIDCProvider struct {
		oauthConfig       *oauth2.Config
		oidcProvider      *oidc.Provider
		oidcVerifier      *oidc.IDTokenVerifier
		oidcEndSessionURL *url.URL
		allowedUsers      []string
		allowedGroups     []string
	}

	providerJSON struct {
		oidc.ProviderConfig
		EndSessionURL string `json:"end_session_endpoint"`
	}
)

const CookieOauthState = "godoxy_oidc_state"

const (
	OIDCAuthCallbackPath = "/auth/callback"
	OIDCPostAuthPath     = "/auth/postauth"
	OIDCLogoutPath       = "/auth/logout"
)

var (
	ErrMissingState = errors.New("missing state cookie")
	ErrInvalidState = errors.New("invalid oauth state")
)

func NewOIDCProvider(issuerURL, clientID, clientSecret string, allowedUsers, allowedGroups []string) (*OIDCProvider, error) {
	if len(allowedUsers)+len(allowedGroups) == 0 {
		return nil, errors.New("OIDC users, groups, or both must not be empty")
	}
	provider, err := oidc.NewProvider(context.Background(), issuerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize OIDC provider: %w", err)
	}

	endSessionURL, err := url.Parse(provider.EndSessionEndpoint())
	if err != nil && provider.EndSessionEndpoint() != "" {
		// non critical, just warn
		logging.Warn().
			Str("issuer", issuerURL).
			Err(err).
			Msg("failed to parse end session URL")
	}

	return &OIDCProvider{
		oauthConfig: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  "",
			Endpoint:     provider.Endpoint(),
			Scopes:       strutils.CommaSeperatedList(common.OIDCScopes),
		},
		oidcProvider: provider,
		oidcVerifier: provider.Verifier(&oidc.Config{
			ClientID: clientID,
		}),
		oidcEndSessionURL: endSessionURL,
		allowedUsers:      allowedUsers,
		allowedGroups:     allowedGroups,
	}, nil
}

// NewOIDCProviderFromEnv creates a new OIDCProvider from environment variables.
func NewOIDCProviderFromEnv() (*OIDCProvider, error) {
	return NewOIDCProvider(
		common.OIDCIssuerURL,
		common.OIDCClientID,
		common.OIDCClientSecret,
		common.OIDCAllowedUsers,
		common.OIDCAllowedGroups,
	)
}

func (auth *OIDCProvider) TokenCookieName() string {
	return "godoxy_oidc_token"
}

func (auth *OIDCProvider) SetAllowedUsers(users []string) {
	auth.allowedUsers = users
}

func (auth *OIDCProvider) SetAllowedGroups(groups []string) {
	auth.allowedGroups = groups
}

func (auth *OIDCProvider) getVerifyStateCookie(r *http.Request) (string, error) {
	state, err := r.Cookie(CookieOauthState)
	if err != nil {
		return "", ErrMissingState
	}
	if r.URL.Query().Get("state") != state.Value {
		return "", ErrInvalidState
	}
	return state.Value, nil
}

func optRedirectPostAuth(r *http.Request) oauth2.AuthCodeOption {
	return oauth2.SetAuthURLParam("redirect_uri", "https://"+r.Host+OIDCPostAuthPath)
}

func (auth *OIDCProvider) HandleAuth(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodHead:
		w.WriteHeader(http.StatusOK)
		return
	case http.MethodGet:
		break
	default:
		gphttp.Forbidden(w, "method not allowed")
		return
	}

	switch r.URL.Path {
	case OIDCAuthCallbackPath:
		state := generateState()
		http.SetCookie(w, &http.Cookie{
			Name:     CookieOauthState,
			Value:    state,
			MaxAge:   300,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   common.APIJWTSecure,
			Path:     "/",
		})
		// redirect user to Idp
		http.Redirect(w, r, auth.oauthConfig.AuthCodeURL(state, optRedirectPostAuth(r)), http.StatusTemporaryRedirect)
	case OIDCPostAuthPath:
		auth.PostAuthCallbackHandler(w, r)
	default:
		auth.LogoutHandler(w, r)
	}
}

func (auth *OIDCProvider) CheckToken(r *http.Request) error {
	token, err := r.Cookie(auth.TokenCookieName())
	if err != nil {
		return ErrMissingToken
	}

	// checks for Expiry, Audience == ClientID, Issuer, etc.
	idToken, err := auth.oidcVerifier.Verify(r.Context(), token.Value)
	if err != nil {
		return fmt.Errorf("failed to verify ID token: %w: %w", ErrInvalidToken, err)
	}

	if len(idToken.Audience) == 0 {
		return ErrInvalidToken
	}

	var claims struct {
		Email    string   `json:"email"`
		Username string   `json:"preferred_username"`
		Groups   []string `json:"groups"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return fmt.Errorf("failed to parse claims: %w", err)
	}

	// Logical AND between allowed users and groups.
	allowedUser := slices.Contains(auth.allowedUsers, claims.Username)
	allowedGroup := len(utils.Intersect(claims.Groups, auth.allowedGroups)) > 0
	if !allowedUser && !allowedGroup {
		return ErrUserNotAllowed
	}
	return nil
}

func (auth *OIDCProvider) RedirectLoginPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

func (auth *OIDCProvider) PostAuthCallbackHandler(w http.ResponseWriter, r *http.Request) {
	// For testing purposes, skip provider verification
	if common.IsTest {
		auth.handleTestCallback(w, r)
		return
	}

	_, err := auth.getVerifyStateCookie(r)
	if err != nil {
		gphttp.BadRequest(w, err.Error())
		return
	}

	code := r.URL.Query().Get("code")
	oauth2Token, err := auth.oauthConfig.Exchange(r.Context(), code, optRedirectPostAuth(r))
	if err != nil {
		gphttp.ServerError(w, r, fmt.Errorf("failed to exchange token: %w", err))
		return
	}

	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		gphttp.BadRequest(w, "missing id_token")
		return
	}

	idToken, err := auth.oidcVerifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		gphttp.ServerError(w, r, fmt.Errorf("failed to verify ID token: %w", err))
		return
	}

	setTokenCookie(w, r, auth.TokenCookieName(), rawIDToken, time.Until(idToken.Expiry))

	// Redirect to home page
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

func (auth *OIDCProvider) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	if auth.oidcEndSessionURL == nil {
		clearTokenCookie(w, r, auth.TokenCookieName())
		http.Redirect(w, r, OIDCAuthCallbackPath, http.StatusTemporaryRedirect)
		return
	}

	token, err := r.Cookie(auth.TokenCookieName())
	if err == nil {
		query := auth.oidcEndSessionURL.Query()
		query.Add("id_token_hint", token.Value)

		logoutURL := *auth.oidcEndSessionURL
		logoutURL.RawQuery = query.Encode()

		clearTokenCookie(w, r, auth.TokenCookieName())
		http.Redirect(w, r, logoutURL.String(), http.StatusFound)
	}

	http.Redirect(w, r, OIDCAuthCallbackPath, http.StatusTemporaryRedirect)
}

// handleTestCallback handles OIDC callback in test environment.
func (auth *OIDCProvider) handleTestCallback(w http.ResponseWriter, r *http.Request) {
	state, err := r.Cookie(CookieOauthState)
	if err != nil {
		gphttp.BadRequest(w, "missing state cookie")
		return
	}

	if r.URL.Query().Get("state") != state.Value {
		gphttp.BadRequest(w, "invalid oauth state")
		return
	}

	// Create test JWT token
	setTokenCookie(w, r, auth.TokenCookieName(), "test", time.Hour)

	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}
