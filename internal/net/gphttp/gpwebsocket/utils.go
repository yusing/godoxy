package gpwebsocket

import (
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yusing/go-proxy/internal/logging"
)

func warnNoMatchDomains() {
	logging.Warn().Msg("no match domains configured, accepting websocket API request from all origins")
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

var localAddresses = []string{"127.0.0.1", "10.0.*.*", "172.16.*.*", "192.168.*.*"}

const writeTimeout = time.Second * 10

func Initiate(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	upgrader := websocket.Upgrader{}

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
			if slices.Contains(localAddresses, host) {
				return true
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

func Periodic(w http.ResponseWriter, r *http.Request, interval time.Duration, do func(conn *websocket.Conn) error) {
	conn, err := Initiate(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	if err := do(conn); err != nil {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if err := do(conn); err != nil {
				return
			}
		}
	}
}

// WriteText writes a text message to the websocket connection.
// It returns true if the message was written successfully, false otherwise.
// It logs an error if the message is not written successfully.
func WriteText(conn *websocket.Conn, msg string) error {
	_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	return conn.WriteMessage(websocket.TextMessage, []byte(msg))
}
