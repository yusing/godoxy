package notif

import (
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/yusing/go-proxy/internal/gperr"
)

type ProviderBase struct {
	Name  string `json:"name" validate:"required"`
	URL   string `json:"url" validate:"url"`
	Token string `json:"token"`
}

var (
	ErrMissingToken     = gperr.New("token is required")
	ErrURLMissingScheme = gperr.New("url missing scheme, expect 'http://' or 'https://'")
)

// Validate implements the utils.CustomValidator interface.
func (base *ProviderBase) Validate() gperr.Error {
	if base.Token == "" {
		return ErrMissingToken
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

func (base *ProviderBase) makeRespError(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err == nil {
		return gperr.Errorf("%s status %d: %s", base.Name, resp.StatusCode, body)
	}
	return gperr.Errorf("%s status %d", base.Name, resp.StatusCode)
}
