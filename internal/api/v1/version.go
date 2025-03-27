package v1

import (
	"net/http"

	"github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/pkg"
)

func GetVersion(w http.ResponseWriter, r *http.Request) {
	gphttp.WriteBody(w, []byte(pkg.GetVersion()))
}
