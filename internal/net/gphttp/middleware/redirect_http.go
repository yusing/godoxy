package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/logging"
)

type redirectHTTP struct {
	Bypass struct {
		UserAgents []string
	}
}

var RedirectHTTP = NewMiddleware[redirectHTTP]()

// before implements RequestModifier.
func (m *redirectHTTP) before(w http.ResponseWriter, r *http.Request) (proceed bool) {
	if r.TLS != nil {
		return true
	}

	if len(m.Bypass.UserAgents) > 0 {
		ua := r.UserAgent()
		for _, uaBypass := range m.Bypass.UserAgents {
			if strings.Contains(ua, uaBypass) {
				return true
			}
		}
	}

	r.URL.Scheme = "https"
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		host = r.Host
	}

	if common.ProxyHTTPSPort != "443" {
		r.URL.Host = host + ":" + common.ProxyHTTPSPort
	} else {
		r.URL.Host = host
	}

	http.Redirect(w, r, r.URL.String(), http.StatusPermanentRedirect)

	logging.Debug().Str("url", r.URL.String()).Str("user_agent", r.UserAgent()).Msg("redirect to https")
	return false
}
