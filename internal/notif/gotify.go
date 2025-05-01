package notif

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gotify/server/v2/model"
	"github.com/rs/zerolog"
)

type (
	GotifyClient struct {
		ProviderBase
	}
	GotifyMessage model.MessageExternal
)

const gotifyMsgEndpoint = "/message"

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

	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// makeRespError implements Provider.
func (client *GotifyClient) makeRespError(resp *http.Response) error {
	var errm model.Error
	err := json.NewDecoder(resp.Body).Decode(&errm)
	if err != nil {
		return fmt.Errorf("%s status %d, but failed to decode err response: %w", client.Name, resp.StatusCode, err)
	}
	return fmt.Errorf("%s status %d %s: %s", client.Name, resp.StatusCode, errm.Error, errm.ErrorDescription)
}
