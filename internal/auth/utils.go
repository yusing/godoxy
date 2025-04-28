package auth

import (
	"net/http"
	"time"

	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

var (
	ErrMissingSessionToken = gperr.New("missing session token")
	ErrInvalidSessionToken = gperr.New("invalid session token")
	ErrUserNotAllowed      = gperr.New("user not allowed")
)

func IsFrontend(r *http.Request) bool {
	return r.Host == common.APIHTTPAddr
}

func requestHost(r *http.Request) string {
	// check if it's from backend
	if IsFrontend(r) {
		return r.Header.Get("X-Forwarded-Host")
	}
	return r.Host
}

// cookieDomain returns the fully qualified domain name of the request host
// with subdomain stripped.
//
// If the request host does not have a subdomain,
// an empty string is returned
//
//	"abc.example.com" -> ".example.com" (cross subdomain)
//	"example.com" -> "" (same domain only)
func cookieDomain(r *http.Request) string {
	parts := strutils.SplitRune(requestHost(r), '.')
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
		Domain:   cookieDomain(r),
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
		Domain:   cookieDomain(r),
		HttpOnly: true,
		Secure:   common.APIJWTSecure,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})
}
