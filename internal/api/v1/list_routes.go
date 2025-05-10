package v1

import (
	"net/http"

	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/route/routes"
	route "github.com/yusing/go-proxy/internal/route/types"
)

func ListRoutesHandler(cfg config.ConfigInstance, w http.ResponseWriter, r *http.Request) {
	gphttp.RespondJSON(w, r, routes.ByAlias(route.RouteType(r.FormValue("type"))))
}
