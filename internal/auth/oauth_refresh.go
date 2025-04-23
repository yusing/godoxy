package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/jsonstore"
	"github.com/yusing/go-proxy/internal/logging"
	"golang.org/x/oauth2"
)

type oauthRefreshToken struct {
	Username     string    `json:"username"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`
}

type Session struct {
	SessionID sessionID `json:"session_id"`
	Username  string    `json:"username"`
	Groups    []string  `json:"groups"`
}

type sessionClaims struct {
	Session
	jwt.RegisteredClaims
}

type sessionID string

var oauthRefreshTokens jsonstore.JSONStore[oauthRefreshToken]

var (
	defaultRefreshTokenExpiry = 30 * 24 * time.Hour // 1 month
	refreshBefore             = 30 * time.Second
)

const sessionTokenIssuer = "GoDoxy"

func init() {
	if IsOIDCEnabled() {
		oauthRefreshTokens = jsonstore.NewStore[oauthRefreshToken]("oauth_refresh_tokens")
	}
}

func (token *oauthRefreshToken) expired() bool {
	return time.Now().After(token.Expiry)
}

func newSessionID() sessionID {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return sessionID(base64.StdEncoding.EncodeToString(b))
}

func newSession(username string, groups []string) Session {
	return Session{
		SessionID: newSessionID(),
		Username:  username,
		Groups:    groups,
	}
}

func getOnceOAuthRefreshToken(claims *Session) (*oauthRefreshToken, bool) {
	token, ok := oauthRefreshTokens.Load(string(claims.SessionID))
	if !ok {
		return nil, false
	}
	invalidateOAuthRefreshToken(claims.SessionID)
	if token.expired() {
		return nil, false
	}
	if claims.Username != token.Username {
		return nil, false
	}
	return &token, true
}

func storeOAuthRefreshToken(sessionID sessionID, username, token string) {
	logging.Debug().Str("username", username).Msg("setting oauth refresh token")
	oauthRefreshTokens.Store(string(sessionID), oauthRefreshToken{
		Username:     username,
		RefreshToken: token,
		Expiry:       time.Now().Add(defaultRefreshTokenExpiry),
	})
}

func invalidateOAuthRefreshToken(sessionID sessionID) {
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
		logging.Err(err).Msg("failed to sign session token")
		return
	}
	setTokenCookie(w, r, CookieOauthSessionToken, signed, common.APIJWTTokenTTL)
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

func (auth *OIDCProvider) TryRefreshToken(w http.ResponseWriter, r *http.Request) error {
	// check for session token
	sessionCookie, err := r.Cookie(CookieOauthSessionToken)
	if err != nil {
		return ErrMissingToken
	}

	// verify the session cookie
	claims, valid, err := auth.parseSessionJWT(sessionCookie.Value)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	if !valid {
		return ErrInvalidToken
	}

	// check if refresh is possible
	refreshToken, ok := getOnceOAuthRefreshToken(&claims.Session)
	if !ok {
		return ErrMissingToken
	}

	if !auth.checkAllowed(claims.Username, claims.Groups) {
		return ErrUserNotAllowed
	}

	// this step refreshes the token
	// see https://cs.opensource.google/go/x/oauth2/+/refs/tags/v0.29.0:oauth2.go;l=313
	newToken, err := auth.oauthConfig.TokenSource(r.Context(), &oauth2.Token{
		RefreshToken: refreshToken.RefreshToken,
	}).Token()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrRefreshTokenFailure, err)
	}

	idTokenJWT, idToken, err := auth.getIdToken(r.Context(), newToken)
	if err != nil {
		return err
	}

	sessionID := newSessionID()

	logging.Debug().Str("username", claims.Username).Time("expiry", newToken.Expiry).Msg("refreshed token")
	storeOAuthRefreshToken(sessionID, claims.Username, newToken.RefreshToken)

	// set new idToken and new sessionToken
	auth.setIDTokenCookie(w, r, idTokenJWT, time.Until(idToken.Expiry))
	auth.setSessionTokenCookie(w, r, Session{
		SessionID: sessionID,
		Username:  claims.Username,
		Groups:    claims.Groups,
	})
	return nil
}
