package v1

import (
	"net/http"
	"time"

	"github.com/yusing/go-proxy/internal/net/gphttp/gpwebsocket"
	"github.com/yusing/go-proxy/internal/route/routes"
)

func Health(w http.ResponseWriter, r *http.Request) {
	gpwebsocket.DynamicJSONHandler(w, r, routes.HealthMap, 1*time.Second)
}
