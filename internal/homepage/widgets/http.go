package widgets

import (
	"net/http"
	"time"

	"github.com/yusing/godoxy/internal/gperr"
)

var HTTPClient = &http.Client{
	Timeout: 10 * time.Second,
}

var ErrHTTPStatus = gperr.New("http status")
