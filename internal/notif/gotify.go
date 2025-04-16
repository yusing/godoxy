package notif

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/yusing/go-proxy/pkg/json"

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

// MakeBody implements Provider.
func (client *GotifyClient) MakeBody(logMsg *LogMessage) (io.Reader, error) {
	var priority int

	switch logMsg.Level {
	case zerolog.WarnLevel:
		priority = 2
	case zerolog.ErrorLevel:
		priority = 5
	case zerolog.FatalLevel, zerolog.PanicLevel:
		priority = 8
	}

	msg := &GotifyMessage{
		Title:    logMsg.Title,
		Message:  formatMarkdown(logMsg.Extras),
		Priority: &priority,
		Extras: map[string]interface{}{
			"client::display": map[string]string{
				"contentType": "text/markdown",
			},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(data), nil
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
