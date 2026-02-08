package notif

import (
	"fmt"
	"io"

	"github.com/bytedance/sonic"
	"github.com/gotify/server/v2/model"
	"github.com/rs/zerolog"
	gperr "github.com/yusing/goutils/errs"
)

type (
	GotifyClient struct {
		ProviderBase
	}
	GotifyMessage model.MessageExternal
)

const gotifyMsgEndpoint = "/message"

func (client *GotifyClient) Validate() error {
	var errs gperr.Builder
	if err := client.ProviderBase.Validate(); err != nil {
		errs.Add(err)
	}
	if client.Token == "" {
		errs.Adds("token is required")
	}
	return errs.Error()
}

func (client *GotifyClient) GetURL() string {
	return client.URL + gotifyMsgEndpoint
}

// MarshalMessage implements Provider.
func (client *GotifyClient) MarshalMessage(logMsg *LogMessage) ([]byte, error) {
	var priority int

	switch logMsg.Level {
	case zerolog.WarnLevel:
		priority = 2
	case zerolog.ErrorLevel:
		priority = 5
	case zerolog.FatalLevel, zerolog.PanicLevel:
		priority = 8
	}

	body, err := logMsg.Body.Format(client.Format)
	if err != nil {
		return nil, err
	}

	msg := &GotifyMessage{
		Title:    logMsg.Title,
		Message:  string(body),
		Priority: &priority,
	}

	if client.Format == LogFormatMarkdown {
		msg.Extras = map[string]interface{}{
			"client::display": map[string]string{
				"contentType": "text/markdown",
			},
		}
	}

	data, err := sonic.Marshal(msg)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// fmtError implements Provider.
func (client *GotifyClient) fmtError(respBody io.Reader) error {
	var errm model.Error
	err := sonic.ConfigDefault.NewDecoder(respBody).Decode(&errm)
	if err != nil {
		return fmt.Errorf("failed to decode err response: %w", err)
	}
	return fmt.Errorf("%s: %s", errm.Error, errm.ErrorDescription)
}
