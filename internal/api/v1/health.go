package v1

import (
	"net/http"
	"time"

	"github.com/yusing/go-proxy/internal/net/gphttp/gpwebsocket"
	"github.com/yusing/go-proxy/internal/route/routes/routequery"
)

func Health(w http.ResponseWriter, r *http.Request) {
	gpwebsocket.DynamicJSONHandler(w, r, routequery.HealthMap, 1*time.Second)
}
