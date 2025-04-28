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
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/utils"
	"golang.org/x/oauth2"
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

const (
	CookieOauthState        = "godoxy_oidc_state"
	CookieOauthToken        = "godoxy_oauth_token"
	CookieOauthSessionToken = "godoxy_session_token"
)

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

func (auth *OIDCProvider) SetAllowedUsers(users []string) {
	auth.allowedUsers = users
}

func (auth *OIDCProvider) SetAllowedGroups(groups []string) {
	auth.allowedGroups = groups
}

// optRedirectPostAuth returns an oauth2 option that sets the "redirect_uri"
// parameter of the authorization URL to the post auth path of the current
// request host.
func optRedirectPostAuth(r *http.Request) oauth2.AuthCodeOption {
	return oauth2.SetAuthURLParam("redirect_uri", "https://"+requestHost(r)+OIDCPostAuthPath)
}

func (auth *OIDCProvider) getIdToken(ctx context.Context, oauthToken *oauth2.Token) (string, *oidc.IDToken, error) {
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

func (auth *OIDCProvider) LoginHandler(w http.ResponseWriter, r *http.Request) {
	// check for session token
	sessionToken, err := r.Cookie(CookieOauthSessionToken)
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
		logging.Err(err).Msg("failed to refresh token")
		auth.clearCookie(w, r)
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	state := generateState()
	setTokenCookie(w, r, CookieOauthState, state, 300*time.Second)
	// redirect user to Idp
	http.Redirect(w, r, auth.oauthConfig.AuthCodeURL(state, optRedirectPostAuth(r)), http.StatusFound)
}

func parseClaims(idToken *oidc.IDToken) (*IDTokenClaims, error) {
	var claim IDTokenClaims
	if err := idToken.Claims(&claim); err != nil {
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}
	if claim.Username == "" {
		return nil, fmt.Errorf("missing username in ID token")
	}
	return &claim, nil
}

func (auth *OIDCProvider) checkAllowed(user string, groups []string) bool {
	userAllowed := slices.Contains(auth.allowedUsers, user)
	if !userAllowed {
		return false
	}
	if len(auth.allowedGroups) == 0 {
		return true
	}
	return len(utils.Intersect(groups, auth.allowedGroups)) > 0
}

func (auth *OIDCProvider) CheckToken(r *http.Request) error {
	tokenCookie, err := r.Cookie(CookieOauthToken)
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
	state, err := r.Cookie(CookieOauthState)
	if err != nil {
		gphttp.BadRequest(w, "missing state cookie")
		return
	}
	if r.URL.Query().Get("state") != state.Value {
		gphttp.BadRequest(w, "invalid oauth state")
		return
	}

	code := r.URL.Query().Get("code")
	oauth2Token, err := auth.oauthConfig.Exchange(r.Context(), code, optRedirectPostAuth(r))
	if err != nil {
		gphttp.ServerError(w, r, fmt.Errorf("failed to exchange token: %w", err))
		return
	}

	idTokenJWT, idToken, err := auth.getIdToken(r.Context(), oauth2Token)
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
	oauthToken, _ := r.Cookie(CookieOauthToken)
	sessionToken, _ := r.Cookie(CookieOauthSessionToken)
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
	setTokenCookie(w, r, CookieOauthToken, jwt, ttl)
}

func (auth *OIDCProvider) clearCookie(w http.ResponseWriter, r *http.Request) {
	clearTokenCookie(w, r, CookieOauthToken)
	clearTokenCookie(w, r, CookieOauthSessionToken)
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
	setTokenCookie(w, r, CookieOauthToken, "test", time.Hour)

	http.Redirect(w, r, "/", http.StatusFound)
}
