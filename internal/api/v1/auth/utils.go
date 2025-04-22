package auth

import (
	"crypto/rand"
	"encoding/base64"
	"net"
	"net/http"
	"time"

	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

var (
	ErrMissingToken   = gperr.New("missing token")
	ErrInvalidToken   = gperr.New("invalid token")
	ErrUserNotAllowed = gperr.New("user not allowed")
)

// cookieFQDN returns the fully qualified domain name of the request host
// with subdomain stripped.
//
// If the request host does not have a subdomain,
// an empty string is returned
//
//	"abc.example.com" -> "example.com"
//	"example.com" -> ""
func cookieFQDN(r *http.Request) string {
	var host string
	// check if it's from backend
	switch r.Host {
	case common.APIHTTPAddr:
		// use XFH
		host = r.Header.Get("X-Forwarded-Host")
	default:
		var err error
		host, _, err = net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
	}

	parts := strutils.SplitRune(host, '.')
	if len(parts) < 2 {
		return ""
	}
	parts[0] = ""
	return strutils.JoinRune(parts, '.')
}

func setTokenCookie(w http.ResponseWriter, r *http.Request, name, value string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		MaxAge:   int(ttl.Seconds()),
		Domain:   cookieFQDN(r),
		HttpOnly: true,
		Secure:   common.APIJWTSecure,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})
}

func clearTokenCookie(w http.ResponseWriter, r *http.Request, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		MaxAge:   -1,
		Domain:   cookieFQDN(r),
		HttpOnly: true,
		Secure:   common.APIJWTSecure,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})
}

// generateState generates a random string for OIDC state.
const oidcStateLength = 32

func generateState() string {
	b := make([]byte, oidcStateLength)
	_, _ = rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)[:oidcStateLength]
}
