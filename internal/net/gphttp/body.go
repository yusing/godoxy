package gphttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/yusing/go-proxy/internal/logging"
)

func WriteBody(w http.ResponseWriter, body []byte) {
	if _, err := w.Write(body); err != nil {
		switch {
		case errors.Is(err, http.ErrHandlerTimeout),
			errors.Is(err, context.DeadlineExceeded):
			logging.Err(err).Msg("timeout writing body")
		default:
			logging.Err(err).Msg("failed to write body")
		}
	}
}

func RespondJSON(w http.ResponseWriter, r *http.Request, data any, code ...int) (canProceed bool) {
	if len(code) > 0 {
		w.WriteHeader(code[0])
	}
	w.Header().Set("Content-Type", "application/json")
	var err error

	switch data := data.(type) {
	case string:
		_, err = w.Write([]byte(fmt.Sprintf("%q", data)))
	case []byte:
		panic("use WriteBody instead")
	default:
		err = json.NewEncoder(w).Encode(data)
	}

	if err != nil {
		LogError(r).Err(err).Msg("failed to encode json")
		return false
	}
	return true
}
