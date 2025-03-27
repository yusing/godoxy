package v1

import (
	"net/http"

	"github.com/yusing/go-proxy/internal/net/gphttp"
)

func Index(w http.ResponseWriter, r *http.Request) {
	gphttp.WriteBody(w, []byte("API ready"))
}
