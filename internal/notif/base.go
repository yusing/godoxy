package notif

import (
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/yusing/go-proxy/internal/gperr"
)

type ProviderBase struct {
	Name   string     `json:"name" validate:"required"`
	URL    string     `json:"url" validate:"url"`
	Token  string     `json:"token"`
	Format *LogFormat `json:"format"`
}

type rawError []byte

func (e rawError) Error() string {
	return string(e)
}

var (
	ErrMissingToken     = gperr.New("token is required")
	ErrURLMissingScheme = gperr.New("url missing scheme, expect 'http://' or 'https://'")
	ErrUnknownError     = gperr.New("unknown error")
)

// Validate implements the utils.CustomValidator interface.
func (base *ProviderBase) Validate() gperr.Error {
	if base.Format == nil || base.Format.string == "" {
		base.Format = LogFormatMarkdown
	}
	if !strings.HasPrefix(base.URL, "http://") && !strings.HasPrefix(base.URL, "https://") {
		return ErrURLMissingScheme
	}
	u, err := url.Parse(base.URL)
	if err != nil {
		return gperr.Wrap(err)
	}
	base.URL = u.String()
	return nil
}

func (base *ProviderBase) GetName() string {
	return base.Name
}

func (base *ProviderBase) GetURL() string {
	return base.URL
}

func (base *ProviderBase) GetToken() string {
	return base.Token
}

func (base *ProviderBase) GetMethod() string {
	return http.MethodPost
}

func (base *ProviderBase) GetMIMEType() string {
	return "application/json"
}

func (base *ProviderBase) SetHeaders(logMsg *LogMessage, headers http.Header) {
	// no-op by default
}

func (base *ProviderBase) fmtError(respBody io.Reader) error {
	body, err := io.ReadAll(respBody)
	if err == nil && len(body) > 0 {
		return rawError(body)
	}
	return ErrUnknownError
}
