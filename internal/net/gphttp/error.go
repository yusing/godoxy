package gphttp

import (
	"context"
	"errors"
	"net/http"
	"syscall"

	"github.com/yusing/godoxy/internal/net/gphttp/httpheaders"
)

// ServerError is for handling server errors.
//
// It logs the error and returns http.StatusInternalServerError to the client.
// Status code can be specified as an argument.
func ServerError(w http.ResponseWriter, r *http.Request, err error, code ...int) {
	switch {
	case err == nil,
		errors.Is(err, context.Canceled),
		errors.Is(err, syscall.EPIPE),
		errors.Is(err, syscall.ECONNRESET):
		return
	}
	LogError(r).Msg(err.Error())
	if httpheaders.IsWebsocket(r.Header) {
		return
	}
	if len(code) == 0 {
		code = []int{http.StatusInternalServerError}
	}
	http.Error(w, http.StatusText(code[0]), code[0])
}
