package v1

import (
	"net/http"

	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/net/gphttp"
)

func Reload(cfg config.ConfigInstance, w http.ResponseWriter, r *http.Request) {
	if err := cfg.Reload(); err != nil {
		gphttp.ServerError(w, r, err)
		return
	}
	gphttp.WriteBody(w, []byte("OK"))
}
