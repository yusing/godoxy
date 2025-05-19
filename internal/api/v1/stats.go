package v1

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/net/gphttp/gpwebsocket"
	"github.com/yusing/go-proxy/internal/net/gphttp/httpheaders"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

func Stats(cfg config.ConfigInstance, w http.ResponseWriter, r *http.Request) {
	if httpheaders.IsWebsocket(r.Header) {
		gpwebsocket.Periodic(w, r, 1*time.Second, func(conn *websocket.Conn) error {
			return conn.WriteJSON(getStats(cfg))
		})
	} else {
		gphttp.RespondJSON(w, r, getStats(cfg))
	}
}

var startTime = time.Now()

func getStats(cfg config.ConfigInstance) map[string]any {
	return map[string]any{
		"proxies": cfg.Statistics(),
		"uptime":  strutils.FormatDuration(time.Since(startTime)),
	}
}
