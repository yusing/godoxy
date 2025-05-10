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

func ListRouteProvidersHandler(cfgInstance config.ConfigInstance, w http.ResponseWriter, r *http.Request) {
	if httpheaders.IsWebsocket(r.Header) {
		gpwebsocket.Periodic(w, r, 5*time.Second, func(conn *websocket.Conn) error {
			return wsjson.Write(r.Context(), conn, cfgInstance.RouteProviderList())
		})
	} else {
		gphttp.RespondJSON(w, r, cfgInstance.RouteProviderList())
	}
}
