package auth

import (
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/yusing/godoxy/internal/common"
	strutils "github.com/yusing/goutils/strings"
)

var (
	ErrMissingSessionToken = errors.New("missing session token")
	ErrInvalidSessionToken = errors.New("invalid session token")
	ErrUserNotAllowed      = errors.New("user not allowed")
)

func IsFrontend(r *http.Request) bool {
	return requestRemoteIP(r) == "127.0.0.1"
}

func requestRemoteIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return ""
	}
	return ip
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
//	"abc.localhost" -> ".localhost"
//	"abc.local" -> ".local"
//	"abc.internal" -> ".internal"
func cookieDomain(r *http.Request) string {
	reqHost := requestHost(r)
	switch {
	case strings.HasSuffix(reqHost, ".internal"):
		return ".internal"
	case strings.HasSuffix(reqHost, ".localhost"):
		return ".localhost"
	case strings.HasSuffix(reqHost, ".local"):
		return ".local"
	}

	// if the host is an IP address, return an empty string
	{
		host, _, err := net.SplitHostPort(reqHost)
		if err != nil {
			host = reqHost
		}
		if net.ParseIP(host) != nil {
			return ""
		}
	}

	parts := strutils.SplitRune(reqHost, '.')
	if len(parts) < 2 {
		return ""
	}
	parts[0] = ""
	return strutils.JoinRune(parts, '.')
}

func SetTokenCookie(w http.ResponseWriter, r *http.Request, name, value string, ttl time.Duration) {
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

func ClearTokenCookie(w http.ResponseWriter, r *http.Request, name string) {
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
