package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/jsonstore"
	"golang.org/x/oauth2"
)

type oauthRefreshToken struct {
	Username     string    `json:"username"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`

	result *RefreshResult
	err    error
	mu     sync.Mutex
}

type Session struct {
	SessionID sessionID `json:"session_id"`
	Username  string    `json:"username"`
	Groups    []string  `json:"groups"`
}

type RefreshResult struct {
	newSession Session
	jwt        string
	jwtExpiry  time.Time
}

type sessionClaims struct {
	Session
	jwt.RegisteredClaims
}

type sessionID string

var oauthRefreshTokens jsonstore.MapStore[*oauthRefreshToken]

var (
	defaultRefreshTokenExpiry = 30 * 24 * time.Hour // 1 month
	sessionInvalidateDelay    = 3 * time.Second
)

var (
	errNoRefreshToken      = errors.New("no refresh token")
	ErrRefreshTokenFailure = errors.New("failed to refresh token")
)

const sessionTokenIssuer = "GoDoxy"

func init() {
	if IsOIDCEnabled() {
		oauthRefreshTokens = jsonstore.Store[*oauthRefreshToken]("oauth_refresh_tokens")
	}
}

func (token *oauthRefreshToken) expired() bool {
	return time.Now().After(token.Expiry)
}

func newSessionID() sessionID {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return sessionID(hex.EncodeToString(b))
}

func newSession(username string, groups []string) Session {
	return Session{
		SessionID: newSessionID(),
		Username:  username,
		Groups:    groups,
	}
}

// getOAuthRefreshToken returns the refresh token for the given session.
func getOAuthRefreshToken(claims *Session) (*oauthRefreshToken, bool) {
	token, ok := oauthRefreshTokens.Load(string(claims.SessionID))
	if !ok {
		return nil, false
	}

	if token.expired() {
		invalidateOAuthRefreshToken(claims.SessionID)
		return nil, false
	}

	if claims.Username != token.Username {
		return nil, false
	}
	return token, true
}

func storeOAuthRefreshToken(sessionID sessionID, username, token string) {
	oauthRefreshTokens.Store(string(sessionID), &oauthRefreshToken{
		Username:     username,
		RefreshToken: token,
		Expiry:       time.Now().Add(defaultRefreshTokenExpiry),
	})
	log.Debug().Str("username", username).Msg("stored oauth refresh token")
}

func invalidateOAuthRefreshToken(sessionID sessionID) {
	log.Debug().Str("session_id", string(sessionID)).Msg("invalidating oauth refresh token")
	oauthRefreshTokens.Delete(string(sessionID))
}

func (auth *OIDCProvider) setSessionTokenCookie(w http.ResponseWriter, r *http.Request, session Session) {
	claims := &sessionClaims{
		Session: session,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    sessionTokenIssuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(common.APIJWTTokenTTL)),
		},
	}
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS512, claims)
	signed, err := jwtToken.SignedString(common.APIJWTSecret)
	if err != nil {
		log.Err(err).Msg("failed to sign session token")
		return
	}
	SetTokenCookie(w, r, auth.getAppScopedCookieName(CookieOauthSessionToken), signed, common.APIJWTTokenTTL)
}

func (auth *OIDCProvider) parseSessionJWT(sessionJWT string) (claims *sessionClaims, valid bool, err error) {
	claims = &sessionClaims{}
	sessionToken, err := jwt.ParseWithClaims(sessionJWT, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return common.APIJWTSecret, nil
	})
	if err != nil {
		return nil, false, err
	}
	return claims, sessionToken.Valid && claims.Issuer == sessionTokenIssuer, nil
}

func (auth *OIDCProvider) TryRefreshToken(ctx context.Context, sessionJWT string) (*RefreshResult, error) {
	// verify the session cookie
	claims, valid, err := auth.parseSessionJWT(sessionJWT)
	if err != nil {
		return nil, fmt.Errorf("session: %s - %w: %w", claims.SessionID, ErrInvalidSessionToken, err)
	}
	if !valid {
		return nil, ErrInvalidSessionToken
	}

	// check if refresh is possible
	refreshToken, ok := getOAuthRefreshToken(&claims.Session)
	if !ok {
		return nil, errNoRefreshToken
	}

	if !auth.checkAllowed(claims.Username, claims.Groups) {
		return nil, ErrUserNotAllowed
	}

	return auth.doRefreshToken(ctx, refreshToken, &claims.Session)
}

func (auth *OIDCProvider) doRefreshToken(ctx context.Context, refreshToken *oauthRefreshToken, claims *Session) (*RefreshResult, error) {
	refreshToken.mu.Lock()
	defer refreshToken.mu.Unlock()

	// already refreshed
	// this must be called after refresh but before invalidate
	if refreshToken.result != nil || refreshToken.err != nil {
		return refreshToken.result, refreshToken.err
	}

	// this step refreshes the token
	// see https://cs.opensource.google/go/x/oauth2/+/refs/tags/v0.29.0:oauth2.go;l=313
	newToken, err := auth.oauthConfig.TokenSource(ctx, &oauth2.Token{
		RefreshToken: refreshToken.RefreshToken,
	}).Token()
	if err != nil {
		refreshToken.err = fmt.Errorf("session: %s - %w: %w", claims.SessionID, ErrRefreshTokenFailure, err)
		return nil, refreshToken.err
	}

	idTokenJWT, idToken, err := auth.getIDToken(ctx, newToken)
	if err != nil {
		refreshToken.err = fmt.Errorf("session: %s - %w: %w", claims.SessionID, ErrRefreshTokenFailure, err)
		return nil, refreshToken.err
	}

	// in case there're multiple requests for the same session to refresh
	// invalidate the token after a short delay
	go func() {
		<-time.After(sessionInvalidateDelay)
		invalidateOAuthRefreshToken(claims.SessionID)
	}()

	sessionID := newSessionID()

	log.Debug().Str("username", claims.Username).Time("expiry", newToken.Expiry).Msg("refreshed token")
	storeOAuthRefreshToken(sessionID, claims.Username, newToken.RefreshToken)

	refreshToken.result = &RefreshResult{
		newSession: Session{
			SessionID: sessionID,
			Username:  claims.Username,
			Groups:    claims.Groups,
		},
		jwt:       idTokenJWT,
		jwtExpiry: idToken.Expiry,
	}
	return refreshToken.result, nil
}
