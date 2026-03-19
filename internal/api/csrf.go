package api

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/auth"
	apitypes "github.com/yusing/goutils/apitypes"
)

// CSRFMiddleware implements the Signed Double Submit Cookie pattern.
//
// Safe methods (GET/HEAD/OPTIONS): ensure a signed CSRF cookie exists.
// Unsafe methods (POST/PUT/DELETE/PATCH): require X-CSRF-Token header
// matching the cookie value, with a valid HMAC signature.
func CSRFMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			ensureCSRFCookie(c)
			c.Next()
			return
		}
		if allowSameOriginAuthBootstrap(c.Request) {
			ensureCSRFCookie(c)
			c.Next()
			return
		}

		cookie, err := c.Request.Cookie(auth.CSRFCookieName)
		if err != nil {
			// No cookie at all — issue one so the frontend can retry.
			reissueCSRFCookie(c)
			c.JSON(http.StatusForbidden, apitypes.Error("missing CSRF token"))
			c.Abort()
			return
		}

		cookieToken := canonicalCSRFToken(cookie.Value)
		headerToken := canonicalCSRFToken(c.GetHeader(auth.CSRFHeaderName))
		if headerToken == "" || cookieToken != headerToken || !auth.ValidateCSRFToken(cookieToken) {
			// Stale or forged token — issue a fresh one so the
			// frontend can read the new cookie and retry.
			reissueCSRFCookie(c)
			c.JSON(http.StatusForbidden, apitypes.Error("invalid CSRF token"))
			c.Abort()
			return
		}

		c.Next()
	}
}

func ensureCSRFCookie(c *gin.Context) {
	if _, err := c.Request.Cookie(auth.CSRFCookieName); err == nil {
		return
	}
	reissueCSRFCookie(c)
}

func reissueCSRFCookie(c *gin.Context) {
	token, err := auth.GenerateCSRFToken()
	if err != nil {
		return
	}
	auth.SetCSRFCookie(c.Writer, c.Request, token)
}

func allowSameOriginAuthBootstrap(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	switch r.URL.Path {
	case "/api/v1/auth/login", "/api/v1/auth/callback":
		return requestSourceMatchesHost(r)
	default:
		return false
	}
}

func requestSourceMatchesHost(r *http.Request) bool {
	for _, header := range []string{"Origin", "Referer"} {
		value := r.Header.Get(header)
		if value == "" {
			continue
		}
		u, err := url.Parse(value)
		if err != nil || u.Host == "" {
			return false
		}
		return normalizeHost(u.Hostname()) == normalizeHost(r.Host)
	}
	return false
}

func normalizeHost(host string) string {
	host = strings.ToLower(host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

func canonicalCSRFToken(token string) string {
	return strings.Trim(strings.TrimSpace(token), "\"")
}
