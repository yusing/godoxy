package auth

import (
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/yusing/godoxy/internal/common"
	"golang.org/x/net/publicsuffix"
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

// cookieDomain is the Domain attribute for auth cookies.
//
// The leftmost label is removed so a session opened on app.example.com is
// also sent to other hosts under example.com. The parent is omitted when it
// is a public suffix ("com", "co.uk", "host", "localhost"): browsers reject
// that Domain, drop the cookie, and the next request looks logged out. The
// cookie is then host-only. IP addresses are host-only too.
//
//	"abc.example.com"    -> ".example.com"
//	"a.b.example.com"    -> ".b.example.com"
//	"example.com"        -> ""
//	"example.host"       -> ""
//	"app.example.host"   -> ".example.host"
//	"example.co.uk"      -> ""
//	"foo.example.co.uk"  -> ".example.co.uk"
//	"abc.localhost"      -> ""
//	"a.b.localhost"      -> ".b.localhost"
//	"192.0.2.1"          -> ""
//	"app.example.com:8443" -> ".example.com"
func cookieDomain(r *http.Request) string {
	host := cookieHost(requestHost(r))
	if host == "" || net.ParseIP(host) != nil || strings.HasPrefix(host, ".") || strings.Contains(host, "..") {
		return ""
	}
	_, parent, ok := strings.Cut(host, ".")
	if !ok || parent == "" || isPublicSuffix(parent) {
		return ""
	}
	return "." + parent
}

// cookieHost is the hostname the browser used, without a port or trailing dot.
// A comma-separated X-Forwarded-Host list uses the first value.
func cookieHost(hostport string) string {
	if before, _, ok := strings.Cut(hostport, ","); ok {
		hostport = before
	}
	hostport = strings.TrimSpace(hostport)
	if host, _, err := net.SplitHostPort(hostport); err == nil {
		hostport = host
	} else {
		hostport = strings.Trim(hostport, "[]")
	}
	return strings.TrimRight(strings.ToLower(hostport), ".")
}

func isPublicSuffix(domain string) bool {
	suffix, _ := publicsuffix.PublicSuffix(domain)
	return suffix == domain
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
