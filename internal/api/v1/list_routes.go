package v1

import (
	"net/http"
	"slices"

	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/route/routes"
)

func ListRoutesHandler(cfg config.ConfigInstance, w http.ResponseWriter, r *http.Request) {
	rts := make([]routes.Route, 0)
	provider := r.FormValue("provider")
	if provider == "" {
		gphttp.RespondJSON(w, r, slices.Collect(routes.Iter))
		return
	}
	for r := range routes.Iter {
		if r.ProviderName() == provider {
			rts = append(rts, r)
		}
	}
	gphttp.RespondJSON(w, r, rts)
}
