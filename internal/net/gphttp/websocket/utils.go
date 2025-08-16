package websocket

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/agent/pkg/agent"
	apitypes "github.com/yusing/go-proxy/internal/api/types"
)

func warnNoMatchDomains() {
	log.Warn().Msg("no match domains configured, accepting websocket API request from all origins")
}

var warnNoMatchDomainOnce sync.Once

const (
	HeaderXGoDoxyWebsocketAllowedDomains = "X-Godoxy-Websocket-Allowed-Domains"
)

func WebsocketAllowedDomains(h http.Header) []string {
	return h[HeaderXGoDoxyWebsocketAllowedDomains]
}

func SetWebsocketAllowedDomains(h http.Header, domains []string) {
	h[HeaderXGoDoxyWebsocketAllowedDomains] = domains
}

// Initiate upgrades the HTTP connection to a WebSocket connection.
// It returns a WebSocket connection and an error if the upgrade fails.
// It logs and responds with an error if the upgrade fails.
//
// No further http.Error should be called after this function.
func Initiate(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	upgrader := websocket.Upgrader{
		Error: errHandler,
	}

	allowedDomains := WebsocketAllowedDomains(r.Header)
	if len(allowedDomains) == 0 {
		warnNoMatchDomainOnce.Do(warnNoMatchDomains)
		upgrader.CheckOrigin = func(r *http.Request) bool {
			return true
		}
	} else {
		upgrader.CheckOrigin = func(r *http.Request) bool {
			host, _, err := net.SplitHostPort(r.Host)
			if err != nil {
				host = r.Host
			}
			if host == "localhost" || host == agent.AgentHost {
				return true
			}
			ip := net.ParseIP(host)
			if ip != nil {
				return ip.IsLoopback() || ip.IsPrivate()
			}
			for _, domain := range allowedDomains {
				if domain[0] == '.' {
					if host == domain[1:] || strings.HasSuffix(host, domain) {
						return true
					}
				} else if host == domain || strings.HasSuffix(host, "."+domain) {
					return true
				}
			}
			return false
		}
	}
	return upgrader.Upgrade(w, r, nil)
}

func PeriodicWrite(c *gin.Context, interval time.Duration, get func() (any, error)) {
	manager, err := NewManagerWithUpgrade(c)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to upgrade to websocket"))
		return
	}
	err = manager.PeriodicWrite(interval, get)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to write to websocket"))
	}
}

func errHandler(w http.ResponseWriter, r *http.Request, status int, reason error) {
	log.Error().
		Str("remote", r.RemoteAddr).
		Str("host", r.Host).
		Str("url", r.URL.String()).
		Int("status", status).
		AnErr("reason", reason).
		Msg("websocket error")
	w.Header().Set("Sec-Websocket-Version", "13")
	http.Error(w, http.StatusText(status), status)
}
