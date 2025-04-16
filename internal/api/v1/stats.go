package v1

import (
	"net/http"
	"time"

	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/net/gphttp/gpwebsocket"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

func Stats(cfg config.ConfigInstance, w http.ResponseWriter, r *http.Request) {
	gpwebsocket.DynamicJSONHandler(w, r, func() map[string]any {
		return map[string]any{
			"proxies": cfg.Statistics(),
			"uptime":  strutils.FormatDuration(time.Since(startTime)),
		}
	}, 1*time.Second)
}

var startTime = time.Now()
