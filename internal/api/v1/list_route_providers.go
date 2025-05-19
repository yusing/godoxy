package v1

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/net/gphttp/gpwebsocket"
	"github.com/yusing/go-proxy/internal/net/gphttp/httpheaders"
)

func ListRouteProvidersHandler(cfgInstance config.ConfigInstance, w http.ResponseWriter, r *http.Request) {
	if httpheaders.IsWebsocket(r.Header) {
		gpwebsocket.Periodic(w, r, 5*time.Second, func(conn *websocket.Conn) error {
			return conn.WriteJSON(cfgInstance.RouteProviderList())
		})
	} else {
		gphttp.RespondJSON(w, r, cfgInstance.RouteProviderList())
	}
}
