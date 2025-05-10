package v1

import (
	"net/http"
	"strconv"

	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/homepage"
	"github.com/yusing/go-proxy/internal/net/gphttp"
)

func ListIconsHandler(cfg config.ConfigInstance, w http.ResponseWriter, r *http.Request) {
	limit, err := strconv.Atoi(r.FormValue("limit"))
	if err != nil {
		limit = 0
	}
	icons, err := homepage.SearchIcons(r.FormValue("keyword"), limit)
	if err != nil {
		gphttp.ClientError(w, r, err)
		return
	}
	gphttp.RespondJSON(w, r, icons)
}
