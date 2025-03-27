package gpwebsocket

import (
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/net/gphttp/httpheaders"
)

func warnNoMatchDomains() {
	logging.Warn().Msg("no match domains configured, accepting websocket API request from all origins")
}

var warnNoMatchDomainOnce sync.Once

func Initiate(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	var originPats []string

	localAddresses := []string{"127.0.0.1", "10.0.*.*", "172.16.*.*", "192.168.*.*"}

	allowedDomains := httpheaders.WebsocketAllowedDomains(r.Header)
	if len(allowedDomains) == 0 || common.IsDebug {
		warnNoMatchDomainOnce.Do(warnNoMatchDomains)
		originPats = []string{"*"}
	} else {
		originPats = make([]string, len(allowedDomains))
		for i, domain := range allowedDomains {
			if domain[0] != '.' {
				originPats[i] = "*." + domain
			} else {
				originPats[i] = "*" + domain
			}
		}
		originPats = append(originPats, localAddresses...)
	}
	return websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: originPats,
	})
}

func Periodic(w http.ResponseWriter, r *http.Request, interval time.Duration, do func(conn *websocket.Conn) error) {
	conn, err := Initiate(w, r)
	if err != nil {
		gphttp.ServerError(w, r, err)
		return
	}
	//nolint:errcheck
	defer conn.CloseNow()

	if err := do(conn); err != nil {
		gphttp.ServerError(w, r, err)
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if err := do(conn); err != nil {
				gphttp.ServerError(w, r, err)
				return
			}
		}
	}
}

// WriteText writes a text message to the websocket connection.
// It returns true if the message was written successfully, false otherwise.
// It logs an error if the message is not written successfully.
func WriteText(r *http.Request, conn *websocket.Conn, msg string) bool {
	if err := conn.Write(r.Context(), websocket.MessageText, []byte(msg)); err != nil {
		gperr.LogError("failed to write text message", err)
		return false
	}
	return true
}
