package v1

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/net/gphttp/gpwebsocket"
	"github.com/yusing/go-proxy/internal/net/gphttp/httpheaders"
)

func ListAgents(w http.ResponseWriter, r *http.Request) {
	if httpheaders.IsWebsocket(r.Header) {
		gpwebsocket.Periodic(w, r, 10*time.Second, func(conn *websocket.Conn) error {
			return conn.WriteJSON(agent.ListAgents())
		})
	} else {
		gphttp.RespondJSON(w, r, agent.ListAgents())
	}
}
