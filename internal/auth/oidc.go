package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/utils"
	"golang.org/x/oauth2"
	"golang.org/x/time/rate"
)

type (
	OIDCProvider struct {
		oauthConfig   *oauth2.Config
		oidcProvider  *oidc.Provider
		oidcVerifier  *oidc.IDTokenVerifier
		endSessionURL *url.URL
		allowedUsers  []string
		allowedGroups []string
	}

	IDTokenClaims struct {
		Username string   `json:"preferred_username"`
		Groups   []string `json:"groups"`
	}
)

var _ Provider = (*OIDCProvider)(nil)

// Cookie names for OIDC authentication
const (
	CookieOauthState        = "godoxy_oidc_state"
	CookieOauthToken        = "godoxy_oauth_token"   //nolint:gosec
	CookieOauthSessionToken = "godoxy_session_token" //nolint:gosec
)

// getAppScopedCookieName returns a cookie name scoped to the specific application
// to prevent conflicts between different OIDC clients
func (auth *OIDCProvider) getAppScopedCookieName(baseName string) string {
	// Use the client ID to scope the cookie name
	// This prevents conflicts when multiple apps use different client IDs
	if auth.oauthConfig.ClientID != "" {
		// Create a hash of the client ID to keep cookie names short
		hash := sha256.Sum256([]byte(auth.oauthConfig.ClientID))
		clientHash := base64.URLEncoding.EncodeToString(hash[:])[:8]
		return fmt.Sprintf("%s_%s", baseName, clientHash)
	}
	return baseName
}

const (
	OIDCAuthInitPath = "/"
	OIDCPostAuthPath = "/auth/callback"
	OIDCLogoutPath   = "/auth/logout"
)

var (
	errMissingIDToken = errors.New("missing id_token field from oauth token")

	ErrMissingOAuthToken = gperr.New("missing oauth token")
	ErrInvalidOAuthToken = gperr.New("invalid oauth token")
)

// generateState generates a random string for OIDC state.
const oidcStateLength = 32

func generateState() string {
	b := make([]byte, oidcStateLength)
	_, _ = rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)[:oidcStateLength]
}

func NewOIDCProvider(issuerURL, clientID, clientSecret string, allowedUsers, allowedGroups []string) (*OIDCProvider, error) {
	if len(allowedUsers)+len(allowedGroups) == 0 {
		return nil, errors.New("oidc.allowed_users or oidc.allowed_groups are both empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize OIDC provider: %w", err)
	}

	endSessionURL, err := url.Parse(provider.EndSessionEndpoint())
	if err != nil && provider.EndSessionEndpoint() != "" {
		// non critical, just warn
		log.Warn().
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
			Scopes:       common.OIDCScopes,
		},
		oidcProvider: provider,
		oidcVerifier: provider.Verifier(&oidc.Config{
			ClientID: clientID,
		}),
		endSessionURL: endSessionURL,
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
		common.OIDCAllowedUsers,
		common.OIDCAllowedGroups,
	)
}

// NewOIDCProviderWithCustomClient creates a new OIDCProvider with custom client credentials
// based on an existing provider (for issuer discovery)
func NewOIDCProviderWithCustomClient(baseProvider *OIDCProvider, clientID, clientSecret string) (*OIDCProvider, error) {
	if clientID == "" || clientSecret == "" {
		return nil, errors.New("client ID and client secret are required")
	}

	// Create a new OIDC verifier with the custom client ID
	oidcVerifier := baseProvider.oidcProvider.Verifier(&oidc.Config{
		ClientID: clientID,
	})

	// Create new OAuth config with custom credentials
	oauthConfig := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  "",
		Endpoint:     baseProvider.oauthConfig.Endpoint,
		Scopes:       baseProvider.oauthConfig.Scopes,
	}

	return &OIDCProvider{
		oauthConfig:   oauthConfig,
		oidcProvider:  baseProvider.oidcProvider,
		oidcVerifier:  oidcVerifier,
		endSessionURL: baseProvider.endSessionURL,
		allowedUsers:  baseProvider.allowedUsers,
		allowedGroups: baseProvider.allowedGroups,
	}, nil
}

func (auth *OIDCProvider) SetAllowedUsers(users []string) {
	auth.allowedUsers = users
}

func (auth *OIDCProvider) SetAllowedGroups(groups []string) {
	auth.allowedGroups = groups
}

func (auth *OIDCProvider) SetScopes(scopes []string) {
	auth.oauthConfig.Scopes = scopes
}

// optRedirectPostAuth returns an oauth2 option that sets the "redirect_uri"
// parameter of the authorization URL to the post auth path of the current
// request host.
func optRedirectPostAuth(r *http.Request) oauth2.AuthCodeOption {
	return oauth2.SetAuthURLParam("redirect_uri", "https://"+requestHost(r)+OIDCPostAuthPath)
}

func (auth *OIDCProvider) getIDToken(ctx context.Context, oauthToken *oauth2.Token) (string, *oidc.IDToken, error) {
	idTokenJWT, ok := oauthToken.Extra("id_token").(string)
	if !ok {
		return "", nil, errMissingIDToken
	}
	idToken, err := auth.oidcVerifier.Verify(ctx, idTokenJWT)
	if err != nil {
		return "", nil, fmt.Errorf("failed to verify ID token: %w", err)
	}
	return idTokenJWT, idToken, nil
}

func (auth *OIDCProvider) HandleAuth(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "" {
		r.URL.Path = OIDCAuthInitPath
	}
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
		r.URL.Scheme = "https"
		http.Redirect(w, r, r.URL.String(), http.StatusFound)
		return
	}
	switch r.URL.Path {
	case OIDCAuthInitPath:
		auth.LoginHandler(w, r)
	case OIDCPostAuthPath:
		auth.PostAuthCallbackHandler(w, r)
	case OIDCLogoutPath:
		auth.LogoutHandler(w, r)
	default:
		http.Redirect(w, r, OIDCAuthInitPath, http.StatusFound)
	}
}

var rateLimit = rate.NewLimiter(rate.Every(time.Second), 1)

func (auth *OIDCProvider) LoginHandler(w http.ResponseWriter, r *http.Request) {
	// check for session token
	sessionToken, err := r.Cookie(auth.getAppScopedCookieName(CookieOauthSessionToken))
	if err == nil { // session token exists
		result, err := auth.TryRefreshToken(r.Context(), sessionToken.Value)
		// redirect back to where they requested
		// when token refresh is ok
		if err == nil {
			auth.setIDTokenCookie(w, r, result.jwt, time.Until(result.jwtExpiry))
			auth.setSessionTokenCookie(w, r, result.newSession)
			ProceedNext(w, r)
			return
		}
		// clear cookies then redirect to home
		log.Err(err).Msg("failed to refresh token")
		auth.clearCookie(w, r)
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if !rateLimit.Allow() {
		http.Error(w, "auth rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	state := generateState()
	SetTokenCookie(w, r, auth.getAppScopedCookieName(CookieOauthState), state, 300*time.Second)
	// redirect user to Idp
	url := auth.oauthConfig.AuthCodeURL(state, optRedirectPostAuth(r))
	if IsFrontend(r) {
		w.Header().Set("X-Redirect-To", url)
		w.WriteHeader(http.StatusForbidden)
	} else {
		http.Redirect(w, r, url, http.StatusFound)
	}
}

func parseClaims(idToken *oidc.IDToken) (*IDTokenClaims, error) {
	var claim IDTokenClaims
	if err := idToken.Claims(&claim); err != nil {
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}
	// Username is optional if groups are present
	if claim.Username == "" && len(claim.Groups) == 0 {
		return nil, errors.New("missing username in ID token")
	}
	return &claim, nil
}

func (auth *OIDCProvider) checkAllowed(user string, groups []string) bool {
	userAllowed := slices.Contains(auth.allowedUsers, user)
	if userAllowed {
		return true
	}
	if len(auth.allowedGroups) == 0 {
		// user is not allowed, but no groups are allowed
		return false
	}
	return len(utils.Intersect(groups, auth.allowedGroups)) > 0
}

func (auth *OIDCProvider) CheckToken(r *http.Request) error {
	tokenCookie, err := r.Cookie(auth.getAppScopedCookieName(CookieOauthToken))
	if err != nil {
		return ErrMissingOAuthToken
	}

	idToken, err := auth.oidcVerifier.Verify(r.Context(), tokenCookie.Value)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidOAuthToken, err)
	}

	claims, err := parseClaims(idToken)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidOAuthToken, err)
	}

	if !auth.checkAllowed(claims.Username, claims.Groups) {
		return ErrUserNotAllowed
	}
	return nil
}

func (auth *OIDCProvider) PostAuthCallbackHandler(w http.ResponseWriter, r *http.Request) {
	// For testing purposes, skip provider verification
	if common.IsTest {
		auth.handleTestCallback(w, r)
		return
	}

	// verify state
	state, err := r.Cookie(auth.getAppScopedCookieName(CookieOauthState))
	if err != nil {
		http.Error(w, "missing state cookie", http.StatusBadRequest)
		return
	}
	if r.URL.Query().Get("state") != state.Value {
		http.Error(w, "invalid oauth state", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	oauth2Token, err := auth.oauthConfig.Exchange(r.Context(), code, optRedirectPostAuth(r))
	if err != nil {
		gphttp.ServerError(w, r, fmt.Errorf("failed to exchange token: %w", err))
		return
	}

	idTokenJWT, idToken, err := auth.getIDToken(r.Context(), oauth2Token)
	if err != nil {
		gphttp.ServerError(w, r, err)
		return
	}

	if oauth2Token.RefreshToken != "" {
		claims, err := parseClaims(idToken)
		if err != nil {
			gphttp.ServerError(w, r, err)
			return
		}
		session := newSession(claims.Username, claims.Groups)
		storeOAuthRefreshToken(session.SessionID, claims.Username, oauth2Token.RefreshToken)
		auth.setSessionTokenCookie(w, r, session)
	}
	auth.setIDTokenCookie(w, r, idTokenJWT, time.Until(idToken.Expiry))

	// Redirect to home page
	http.Redirect(w, r, "/", http.StatusFound)
}

func (auth *OIDCProvider) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	oauthToken, _ := r.Cookie(auth.getAppScopedCookieName(CookieOauthToken))
	sessionToken, _ := r.Cookie(auth.getAppScopedCookieName(CookieOauthSessionToken))
	auth.clearCookie(w, r)

	if sessionToken != nil {
		claims, _, err := auth.parseSessionJWT(sessionToken.Value)
		if err == nil {
			invalidateOAuthRefreshToken(claims.SessionID)
		}
	}

	url := "/"
	if auth.endSessionURL != nil && oauthToken != nil {
		query := auth.endSessionURL.Query()
		query.Set("id_token_hint", oauthToken.Value)
		query.Set("post_logout_redirect_uri", "https://"+requestHost(r))

		clone := *auth.endSessionURL
		clone.RawQuery = query.Encode()
		url = clone.String()
	} else if auth.endSessionURL != nil {
		url = auth.endSessionURL.String()
	}

	http.Redirect(w, r, url, http.StatusFound)
}

func (auth *OIDCProvider) setIDTokenCookie(w http.ResponseWriter, r *http.Request, jwt string, ttl time.Duration) {
	SetTokenCookie(w, r, auth.getAppScopedCookieName(CookieOauthToken), jwt, ttl)
}

func (auth *OIDCProvider) clearCookie(w http.ResponseWriter, r *http.Request) {
	ClearTokenCookie(w, r, auth.getAppScopedCookieName(CookieOauthToken))
	ClearTokenCookie(w, r, auth.getAppScopedCookieName(CookieOauthSessionToken))
}

// handleTestCallback handles OIDC callback in test environment.
func (auth *OIDCProvider) handleTestCallback(w http.ResponseWriter, r *http.Request) {
	state, err := r.Cookie(auth.getAppScopedCookieName(CookieOauthState))
	if err != nil {
		http.Error(w, "missing state cookie", http.StatusBadRequest)
		return
	}

	if r.URL.Query().Get("state") != state.Value {
		http.Error(w, "invalid oauth state", http.StatusBadRequest)
		return
	}

	// Create test JWT token
	SetTokenCookie(w, r, auth.getAppScopedCookieName(CookieOauthToken), "test", time.Hour)

	http.Redirect(w, r, "/", http.StatusFound)
}
