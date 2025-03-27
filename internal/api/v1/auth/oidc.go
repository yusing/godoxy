package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/utils"
	"github.com/yusing/go-proxy/internal/utils/strutils"
	"golang.org/x/oauth2"
)

type OIDCProvider struct {
	oauthConfig   *oauth2.Config
	oidcProvider  *oidc.Provider
	oidcVerifier  *oidc.IDTokenVerifier
	oidcLogoutURL *url.URL
	allowedUsers  []string
	allowedGroups []string
	isMiddleware  bool
}

const CookieOauthState = "godoxy_oidc_state"

const (
	OIDCMiddlewareCallbackPath = "/auth/callback"
	OIDCLogoutPath             = "/auth/logout"
)

func NewOIDCProvider(issuerURL, clientID, clientSecret, redirectURL, logoutURL string, allowedUsers, allowedGroups []string) (*OIDCProvider, error) {
	if len(allowedUsers)+len(allowedGroups) == 0 {
		return nil, errors.New("OIDC users, groups, or both must not be empty")
	}

	var logout *url.URL
	var err error
	if logoutURL != "" {
		logout, err = url.Parse(logoutURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse logout URL: %w", err)
		}
	}

	provider, err := oidc.NewProvider(context.Background(), issuerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize OIDC provider: %w", err)
	}

	return &OIDCProvider{
		oauthConfig: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Endpoint:     provider.Endpoint(),
			Scopes:       strutils.CommaSeperatedList(common.OIDCScopes),
		},
		oidcProvider: provider,
		oidcVerifier: provider.Verifier(&oidc.Config{
			ClientID: clientID,
		}),
		oidcLogoutURL: logout,
		allowedUsers:  allowedUsers,
		allowedGroups: allowedGroups,
	}, nil
}

// NewOIDCProviderFromEnv creates a new OIDCProvider from environment variables.
func NewOIDCProviderFromEnv() (*OIDCProvider, error) {
	return NewOIDCProvider(
		common.OIDCIssuerURL,
		common.OIDCClientID,
		common.OIDCClientSecret,
		common.OIDCRedirectURL,
		common.OIDCLogoutURL,
		common.OIDCAllowedUsers,
		common.OIDCAllowedGroups,
	)
}

func (auth *OIDCProvider) TokenCookieName() string {
	return "godoxy_oidc_token"
}

func (auth *OIDCProvider) SetIsMiddleware(enabled bool) {
	auth.isMiddleware = enabled
	auth.oauthConfig.RedirectURL = ""
}

func (auth *OIDCProvider) SetAllowedUsers(users []string) {
	auth.allowedUsers = users
}

func (auth *OIDCProvider) SetAllowedGroups(groups []string) {
	auth.allowedGroups = groups
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

// generateState generates a random string for OIDC state.
const oidcStateLength = 32

func generateState() (string, error) {
	b := make([]byte, oidcStateLength)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b)[:oidcStateLength], nil
}

// RedirectOIDC initiates the OIDC login flow.
func (auth *OIDCProvider) RedirectLoginPage(w http.ResponseWriter, r *http.Request) {
	state, err := generateState()
	if err != nil {
		gphttp.ServerError(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieOauthState,
		Value:    state,
		MaxAge:   300,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
		Path:     "/",
	})

	redirURL := auth.oauthConfig.AuthCodeURL(state)
	if auth.isMiddleware {
		u, err := r.URL.Parse(redirURL)
		if err != nil {
			gphttp.ServerError(w, r, err)
			return
		}
		q := u.Query()
		q.Set("redirect_uri", "https://"+r.Host+OIDCMiddlewareCallbackPath+q.Get("redirect_uri"))
		u.RawQuery = q.Encode()
		redirURL = u.String()
	}
	http.Redirect(w, r, redirURL, http.StatusTemporaryRedirect)
}

func (auth *OIDCProvider) exchange(r *http.Request) (*oauth2.Token, error) {
	if auth.isMiddleware {
		cfg := *auth.oauthConfig
		cfg.RedirectURL = "https://" + r.Host + OIDCMiddlewareCallbackPath
		return cfg.Exchange(r.Context(), r.URL.Query().Get("code"))
	}
	return auth.oauthConfig.Exchange(r.Context(), r.URL.Query().Get("code"))
}

// OIDCCallbackHandler handles the OIDC callback.
func (auth *OIDCProvider) LoginCallbackHandler(w http.ResponseWriter, r *http.Request) {
	// For testing purposes, skip provider verification
	if common.IsTest {
		auth.handleTestCallback(w, r)
		return
	}

	state, err := r.Cookie(CookieOauthState)
	if err != nil {
		gphttp.BadRequest(w, "missing state cookie")
		return
	}

	query := r.URL.Query()
	if query.Get("state") != state.Value {
		gphttp.BadRequest(w, "invalid oauth state")
		return
	}

	oauth2Token, err := auth.exchange(r)
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

func (auth *OIDCProvider) LogoutCallbackHandler(w http.ResponseWriter, r *http.Request) {
	if auth.oidcLogoutURL == nil {
		DefaultLogoutCallbackHandler(auth, w, r)
		return
	}

	token, err := r.Cookie(auth.TokenCookieName())
	if err != nil {
		gphttp.BadRequest(w, "missing token cookie")
		return
	}
	clearTokenCookie(w, r, auth.TokenCookieName())

	logoutURL := *auth.oidcLogoutURL
	logoutURL.Query().Add("id_token_hint", token.Value)

	http.Redirect(w, r, logoutURL.String(), http.StatusFound)
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
