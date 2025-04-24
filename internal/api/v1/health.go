package v1

import (
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/net/gphttp/gpwebsocket"
	"github.com/yusing/go-proxy/internal/net/gphttp/httpheaders"
	"github.com/yusing/go-proxy/internal/route/routes"
)

func Health(w http.ResponseWriter, r *http.Request) {
	if httpheaders.IsWebsocket(r.Header) {
		gpwebsocket.Periodic(w, r, 1*time.Second, func(conn *websocket.Conn) error {
			return wsjson.Write(r.Context(), conn, routes.HealthMap())
		})
	} else {
		gphttp.RespondJSON(w, r, routes.HealthMap())
	}
}
