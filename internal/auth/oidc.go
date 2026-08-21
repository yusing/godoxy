package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/utils"
	httputils "github.com/yusing/goutils/http"
	"golang.org/x/oauth2"
	"golang.org/x/time/rate"
)

type (
	OIDCProvider struct {
		hash      string // unique identity: sha256(issuer_url\nclient_id)
		issuerURL string

		oauthConfig   *oauth2.Config
		oidcProvider  *oidc.Provider
		oidcVerifier  *oidc.IDTokenVerifier
		endSessionURL *url.URL
		allowedUsers  []string
		allowedGroups []string

		rateLimit *rate.Limiter

		onUnknownPathHandler func(http.ResponseWriter, *http.Request) LoginResult
	}

	IDTokenClaims struct {
		Username string   `json:"preferred_username"`
		Groups   []string `json:"groups"`
	}

	oidcLoginTransactionClaims struct {
		State    string `json:"state"`
		ReturnTo string `json:"return_to"`
		jwt.RegisteredClaims
	}
)

var _ Provider = (*OIDCProvider)(nil)

// Cookie names for OIDC authentication
const (
	CookieOauthState        = "godoxy_oidc_state"
	CookieOauthToken        = "godoxy_oauth_token"   //nolint:gosec
	CookieOauthSessionToken = "godoxy_session_token" //nolint:gosec
)

const (
	oidcLoginCookieTTL         = 5 * time.Minute
	oidcLoginTransactionIssuer = "GoDoxy OIDC Login"
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
	OIDCAuthBasePath = "/auth/"
	OIDCPostAuthPath = OIDCAuthBasePath + "callback"
	OIDCLogoutPath   = OIDCAuthBasePath + "logout"
)

var (
	errMissingIDToken = errors.New("oidc: missing id_token field from oauth token")

	ErrMissingOAuthToken = errors.New("oidc: missing oauth token")
	ErrInvalidOAuthToken = errors.New("oidc: invalid oauth token")
)

// generateState generates a random string for OIDC state.
const oidcStateLength = 32

func generateState() string {
	b := make([]byte, oidcStateLength)
	_, _ = rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)[:oidcStateLength]
}

// OIDCProviderHash returns the identity of an issuer and client pair.
func OIDCProviderHash(issuerURL, clientID string) string {
	hasher := sha256.New()
	fmt.Fprintf(hasher, "%s\n%s", issuerURL, clientID)

	var hash [sha256.Size]byte
	return hex.EncodeToString(hasher.Sum(hash[:0]))
}

func newOIDCProviderFromGlobal(global *OIDCProvider, clientSecret string, scopes, allowedUsers, allowedGroups []string) *OIDCProvider {
	oauthConfig := *global.oauthConfig
	oauthConfig.ClientSecret = clientSecret
	oauthConfig.Scopes = scopes

	return &OIDCProvider{
		hash:          global.hash,
		issuerURL:     global.issuerURL,
		oauthConfig:   &oauthConfig,
		oidcProvider:  global.oidcProvider,
		oidcVerifier:  global.oidcVerifier,
		endSessionURL: global.endSessionURL,
		allowedUsers:  allowedUsers,
		allowedGroups: allowedGroups,
		rateLimit:     global.rateLimit,
	}
}

// NewOIDCProvider initializes an OIDC provider, reusing matching global
// discovery state while preserving per-route authorization settings.
func NewOIDCProvider(ctx context.Context, issuerURL, clientID, clientSecret string, scopes, allowedUsers, allowedGroups []string) (*OIDCProvider, error) {
	if len(allowedUsers)+len(allowedGroups) == 0 {
		return nil, errors.New("oidc: allowed_users or allowed_groups are both empty")
	}

	hash := OIDCProviderHash(issuerURL, clientID)
	if global := globalOIDCProvider(); global != nil && global.hash == hash {
		return newOIDCProviderFromGlobal(global, clientSecret, scopes, allowedUsers, allowedGroups), nil
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("oidc: failed to initialize OIDC provider: %w", err)
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
		hash:      hash,
		issuerURL: issuerURL,
		oauthConfig: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  "",
			Endpoint:     provider.Endpoint(),
			Scopes:       scopes,
		},
		oidcProvider: provider,
		oidcVerifier: provider.Verifier(&oidc.Config{
			ClientID: clientID,
		}),
		endSessionURL: endSessionURL,
		allowedUsers:  allowedUsers,
		allowedGroups: allowedGroups,
		rateLimit:     rate.NewLimiter(rate.Every(common.OIDCRateLimitPeriod), common.OIDCRateLimit),
	}, nil
}

// NewOIDCProviderFromEnv creates a new OIDCProvider from environment variables.
func NewOIDCProviderFromEnv(ctx context.Context) (*OIDCProvider, error) {
	return NewOIDCProvider(
		ctx,
		common.OIDCIssuerURL,
		common.OIDCClientID,
		common.OIDCClientSecret,
		common.OIDCScopes,
		common.OIDCAllowedUsers,
		common.OIDCAllowedGroups,
	)
}

// NewOIDCProviderWithCustomClient creates a new OIDCProvider with custom client credentials
// based on an existing provider (for issuer discovery)
func NewOIDCProviderWithCustomClient(baseProvider *OIDCProvider, clientID, clientSecret string) (*OIDCProvider, error) {
	if clientID == "" || clientSecret == "" {
		return nil, errors.New("oidc: client ID and client secret are required")
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
		hash:          OIDCProviderHash(baseProvider.issuerURL, clientID),
		issuerURL:     baseProvider.issuerURL,
		oauthConfig:   oauthConfig,
		oidcProvider:  baseProvider.oidcProvider,
		oidcVerifier:  oidcVerifier,
		endSessionURL: baseProvider.endSessionURL,
		allowedUsers:  baseProvider.allowedUsers,
		allowedGroups: baseProvider.allowedGroups,
		rateLimit:     baseProvider.rateLimit,
	}, nil
}

func (auth *OIDCProvider) Hash() string {
	return auth.hash
}

func (auth *OIDCProvider) SetOnUnknownPathHandler(handler func(http.ResponseWriter, *http.Request) LoginResult) {
	auth.onUnknownPathHandler = handler
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
		return "", nil, fmt.Errorf("oidc: failed to verify ID token: %w", err)
	}
	return idTokenJWT, idToken, nil
}

func (auth *OIDCProvider) HandleAuth(w http.ResponseWriter, r *http.Request) LoginResult {
	if r.URL.Path == "" {
		r.URL.Path = OIDCAuthInitPath
	}
	if r.TLS == nil && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		r.URL.Scheme = "https"
		http.Redirect(w, r, r.URL.String(), http.StatusFound)
		return LoginResponseHandled
	}
	switch r.URL.Path {
	case OIDCAuthInitPath:
		return auth.LoginHandler(w, r)
	case OIDCPostAuthPath:
		auth.PostAuthCallbackHandler(w, r)
	case OIDCLogoutPath:
		auth.LogoutHandler(w, r)
	default:
		if auth.onUnknownPathHandler != nil {
			return auth.onUnknownPathHandler(w, r)
		}
		http.Redirect(w, r, OIDCAuthInitPath, http.StatusFound)
	}
	return LoginResponseHandled
}

func (auth *OIDCProvider) LoginHandler(w http.ResponseWriter, r *http.Request) LoginResult {
	if !httputils.GetAccept(r.Header).AcceptHTML() {
		http.Error(w, "authentication is required", http.StatusForbidden)
		return LoginResponseHandled
	}

	if err := auth.refreshSession(w, r); err == nil {
		// The caller owns the response after refresh because document, API, and
		// protocol requests require different completion behavior.
		return LoginSessionRefreshed
	} else if !errors.Is(err, ErrMissingSessionToken) {
		// Discard the invalid session and restart login for this document.
		log.Err(err).Msg("failed to refresh token")
	}
	return auth.startLogin(w, r)
}

func (auth *OIDCProvider) refreshSession(w http.ResponseWriter, r *http.Request) error {
	sessionToken, err := r.Cookie(auth.getAppScopedCookieName(CookieOauthSessionToken))
	if err != nil {
		return ErrMissingSessionToken
	}
	result, err := auth.TryRefreshToken(r.Context(), sessionToken.Value)
	if err != nil {
		auth.clearCookie(w, r)
		return err
	}
	auth.setIDTokenCookie(w, r, result.jwt, time.Until(result.jwtExpiry))
	auth.setSessionTokenCookie(w, r, result.newSession)
	return nil
}

func (auth *OIDCProvider) startLogin(w http.ResponseWriter, r *http.Request) LoginResult {
	if !auth.rateLimit.Allow() {
		WriteBlockPage(w, http.StatusTooManyRequests, "auth rate limit exceeded", "Try again", OIDCAuthInitPath)
		return LoginResponseHandled
	}

	state := generateState()
	if err := auth.setLoginTransactionCookie(w, r, state); err != nil {
		WriteBlockPage(w, http.StatusInternalServerError, "failed to start oauth login", "Try again", OIDCAuthInitPath)
		log.Err(err).Msg("failed to sign oauth login transaction")
		return LoginResponseHandled
	}
	// redirect user to Idp
	url := auth.oauthConfig.AuthCodeURL(state, optRedirectPostAuth(r))
	http.Redirect(w, r, url, http.StatusFound)
	return LoginResponseHandled
}

func parseClaims(idToken *oidc.IDToken) (*IDTokenClaims, error) {
	var claim IDTokenClaims
	if err := idToken.Claims(&claim); err != nil {
		return nil, fmt.Errorf("oidc: failed to parse claims: %w", err)
	}
	// Username is optional if groups are present
	if claim.Username == "" && len(claim.Groups) == 0 {
		return nil, errors.New("oidc: missing username in ID token")
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

	state := r.URL.Query().Get("state")
	returnTo, err := auth.consumeLoginTransaction(w, r, state)
	if err != nil {
		auth.clearCookie(w, r)
		WriteBlockPage(w, http.StatusBadRequest, "invalid oauth state", "Back to Login", OIDCAuthInitPath)
		return
	}

	code := r.URL.Query().Get("code")
	oauth2Token, err := auth.oauthConfig.Exchange(r.Context(), code, optRedirectPostAuth(r))
	if err != nil {
		auth.clearCookie(w, r)
		WriteBlockPage(w, http.StatusInternalServerError, "failed to exchange token", "Try again", OIDCAuthInitPath)
		httputils.LogError(r).Msgf("failed to exchange token: %v", err)
		return
	}

	idTokenJWT, idToken, err := auth.getIDToken(r.Context(), oauth2Token)
	if err != nil {
		auth.clearCookie(w, r)
		WriteBlockPage(w, http.StatusInternalServerError, "failed to get ID token", "Try again", OIDCAuthInitPath)
		httputils.LogError(r).Msgf("failed to get ID token: %v", err)
		return
	}

	if oauth2Token.RefreshToken != "" {
		claims, err := parseClaims(idToken)
		if err != nil {
			auth.clearCookie(w, r)
			WriteBlockPage(w, http.StatusInternalServerError, "failed to parse claims", "Try again", OIDCAuthInitPath)
			httputils.LogError(r).Msgf("failed to parse claims: %v", err)
			return
		}
		session := newSession(claims.Username, claims.Groups)
		storeOAuthRefreshToken(session.SessionID, claims.Username, oauth2Token.RefreshToken)
		auth.setSessionTokenCookie(w, r, session)
	}
	auth.setIDTokenCookie(w, r, idTokenJWT, time.Until(idToken.Expiry))

	http.Redirect(w, r, returnTo, http.StatusFound)
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

func (auth *OIDCProvider) loginTransactionCookieName(state string) string {
	return auth.getAppScopedCookieName(CookieOauthState + "_" + state)
}

func (auth *OIDCProvider) setLoginTransactionCookie(w http.ResponseWriter, r *http.Request, state string) error {
	returnTo := "/"
	if r.Method == http.MethodGet {
		returnTo = localRequestURI(r)
	}
	now := time.Now()
	claims := oidcLoginTransactionClaims{
		State:     state,
		ReturnTo:  returnTo,
		Issuer:    oidcLoginTransactionIssuer,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(oidcLoginCookieTTL)),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS512, claims).SignedString(common.APIJWTSecret)
	if err != nil {
		return err
	}
	SetTokenCookie(w, r, auth.loginTransactionCookieName(state), signed, oidcLoginCookieTTL)
	return nil
}

func (auth *OIDCProvider) consumeLoginTransaction(w http.ResponseWriter, r *http.Request, state string) (string, error) {
	if !validOIDCState(state) {
		return "/", errors.New("invalid oauth state format")
	}
	cookieName := auth.loginTransactionCookieName(state)
	defer ClearTokenCookie(w, r, cookieName)
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return "/", err
	}
	claims := &oidcLoginTransactionClaims{}
	token, err := jwt.ParseWithClaims(cookie.Value, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS512 {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return common.APIJWTSecret, nil
	}, jwt.WithIssuer(oidcLoginTransactionIssuer), jwt.WithValidMethods([]string{jwt.SigningMethodHS512.Alg()}))
	if err != nil {
		return "/", err
	}
	if !token.Valid || claims.State != state {
		return "/", errors.New("oauth login transaction does not match state")
	}
	raw := claims.ReturnTo
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return "/", nil
	}
	target, err := url.ParseRequestURI(raw)
	if err != nil || target.IsAbs() || target.Host != "" {
		return "/", nil
	}
	return localRequestURI(&http.Request{URL: target}), nil
}

func validOIDCState(state string) bool {
	if len(state) != oidcStateLength {
		return false
	}
	return strings.IndexFunc(state, func(r rune) bool {
		return !('a' <= r && r <= 'z') &&
			!('A' <= r && r <= 'Z') &&
			!('0' <= r && r <= '9') &&
			r != '-' && r != '_'
	}) == -1
}

// handleTestCallback handles OIDC callback in test environment.
func (auth *OIDCProvider) handleTestCallback(w http.ResponseWriter, r *http.Request) {
	returnTo, err := auth.consumeLoginTransaction(w, r, r.URL.Query().Get("state"))
	if err != nil {
		http.Error(w, "invalid oauth state", http.StatusBadRequest)
		return
	}

	// Create test JWT token
	SetTokenCookie(w, r, auth.getAppScopedCookieName(CookieOauthToken), "test", time.Hour)

	http.Redirect(w, r, returnTo, http.StatusFound)
}
