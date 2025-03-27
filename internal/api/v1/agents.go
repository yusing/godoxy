package v1

import (
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/net/gphttp/gpwebsocket"
	"github.com/yusing/go-proxy/internal/net/gphttp/httpheaders"
)

func ListAgents(cfg config.ConfigInstance, w http.ResponseWriter, r *http.Request) {
	if httpheaders.IsWebsocket(r.Header) {
		gpwebsocket.Periodic(w, r, 10*time.Second, func(conn *websocket.Conn) error {
			wsjson.Write(r.Context(), conn, cfg.ListAgents())
			return nil
		})
	} else {
		gphttp.RespondJSON(w, r, cfg.ListAgents())
	}
}
