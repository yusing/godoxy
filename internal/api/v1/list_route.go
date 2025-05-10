package v1

import (
	"net/http"

	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/route/routes"
)

func ListRouteHandler(cfg config.ConfigInstance, w http.ResponseWriter, r *http.Request) {
	which := r.PathValue("which")
	route, ok := routes.Get(which)
	if ok {
		gphttp.RespondJSON(w, r, route)
	} else {
		gphttp.RespondJSON(w, r, nil)
	}
}
