package notif

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	gperr "github.com/yusing/goutils/errs"
)

type ProviderBase struct {
	Name   string    `json:"name" validate:"required"`
	URL    string    `json:"url" validate:"url"`
	Token  string    `json:"token"`
	Format LogFormat `json:"format"`
}

type rawError []byte

func (e rawError) Error() string {
	return string(e)
}

var (
	ErrMissingToken     = errors.New("token is required")
	ErrURLMissingScheme = errors.New("url missing scheme, expect 'http://' or 'https://'")
	ErrUnknownError     = errors.New("unknown error")
)

// Validate implements the utils.CustomValidator interface.
func (base *ProviderBase) Validate() error {
	switch base.Format {
	case "":
		base.Format = LogFormatMarkdown
	case LogFormatPlain, LogFormatMarkdown:
	default:
		return gperr.Multiline().
			Addf("invalid log format %s, supported formats:", base.Format).
			AddLines(
				LogFormatPlain,
				LogFormatMarkdown,
			)
	}

	if !strings.HasPrefix(base.URL, "http://") && !strings.HasPrefix(base.URL, "https://") {
		return ErrURLMissingScheme
	}
	u, err := url.Parse(base.URL)
	if err != nil {
		return err
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
