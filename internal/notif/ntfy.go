package notif

import (
	"net/http"

	"github.com/rs/zerolog"
	"github.com/yusing/godoxy/internal/gperr"
)

// See https://docs.ntfy.sh/publish
type Ntfy struct {
	ProviderBase
	Topic string `json:"topic"`
}

// Validate implements the utils.CustomValidator interface.
func (n *Ntfy) Validate() gperr.Error {
	if err := n.ProviderBase.Validate(); err != nil {
		return err
	}
	if n.URL == "" {
		return gperr.New("url is required")
	}
	if n.Topic == "" {
		return gperr.New("topic is required")
	}
	if n.Topic[0] == '/' {
		return gperr.New("topic should not start with a slash")
	}
	return nil
}

// GetURL implements Provider.
func (n *Ntfy) GetURL() string {
	if n.URL[len(n.URL)-1] == '/' {
		return n.URL + n.Topic
	}
	return n.URL + "/" + n.Topic
}

// GetMIMEType implements Provider.
func (n *Ntfy) GetMIMEType() string {
	return ""
}

// GetToken implements Provider.
func (n *Ntfy) GetToken() string {
	return n.Token
}

// MarshalMessage implements Provider.
func (n *Ntfy) MarshalMessage(logMsg *LogMessage) ([]byte, error) {
	return logMsg.Body.Format(n.Format)
}

// SetHeaders implements Provider.
func (n *Ntfy) SetHeaders(logMsg *LogMessage, headers http.Header) {
	headers.Set("Title", logMsg.Title)

	switch logMsg.Level {
	// warning (or other unspecified) uses default priority
	case zerolog.FatalLevel:
		headers.Set("Priority", "urgent")
	case zerolog.ErrorLevel:
		headers.Set("Priority", "high")
	case zerolog.InfoLevel:
		headers.Set("Priority", "low")
	case zerolog.DebugLevel:
		headers.Set("Priority", "min")
	}

	if n.Format == LogFormatMarkdown {
		headers.Set("Markdown", "yes")
	}
}
