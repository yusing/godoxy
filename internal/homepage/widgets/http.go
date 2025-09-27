package widgets

import (
	"net/http"
	"time"

	gperr "github.com/yusing/goutils/errs"
)

var HTTPClient = &http.Client{
	Timeout: 10 * time.Second,
}

var ErrHTTPStatus = gperr.New("http status")
