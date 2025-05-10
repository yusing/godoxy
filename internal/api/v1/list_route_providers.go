package v1

import (
	"net/http"

	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/net/gphttp"
)

func ListRouteProvidersHandler(cfg config.ConfigInstance, w http.ResponseWriter, r *http.Request) {
	gphttp.RespondJSON(w, r, cfg.RouteProviderList())
}
