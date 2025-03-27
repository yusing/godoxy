package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/yusing/go-proxy/internal/net/gphttp/httpheaders"
)

type (
	setXForwarded  struct{}
	hideXForwarded struct{}
)

var (
	SetXForwarded  = NewMiddleware[setXForwarded]()
	HideXForwarded = NewMiddleware[hideXForwarded]()
)

// before implements RequestModifier.
func (setXForwarded) before(w http.ResponseWriter, r *http.Request) (proceed bool) {
	r.Header.Del(httpheaders.HeaderXForwardedFor)
	clientIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		r.Header.Set(httpheaders.HeaderXForwardedFor, clientIP)
	}
	return true
}

// before implements RequestModifier.
func (hideXForwarded) before(w http.ResponseWriter, r *http.Request) (proceed bool) {
	toDelete := make([]string, 0, len(r.Header))
	for k := range r.Header {
		if strings.HasPrefix(k, "X-Forwarded-") {
			toDelete = append(toDelete, k)
		}
	}

	for _, k := range toDelete {
		r.Header.Del(k)
	}

	return true
}
