package widgets

import (
	"errors"
	"net/http"
	"time"
)

var HTTPClient = &http.Client{
	Timeout: 10 * time.Second,
}

var ErrHTTPStatus = errors.New("http status")
