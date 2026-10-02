package notif

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestGotifyMarshalMessagePriorities(t *testing.T) {
	client := &GotifyClient{ProviderBase: ProviderBase{Format: LogFormatPlain}}
	for _, tt := range []struct {
		level    zerolog.Level
		priority int
	}{
		{zerolog.TraceLevel, 0},
		{zerolog.DebugLevel, 0},
		{zerolog.InfoLevel, 0},
		{zerolog.NoLevel, 0},
		{zerolog.WarnLevel, 2},
		{zerolog.ErrorLevel, 5},
		{zerolog.FatalLevel, 8},
		{zerolog.PanicLevel, 8},
	} {
		t.Run(tt.level.String(), func(t *testing.T) {
			data, err := client.MarshalMessage(&LogMessage{
				Title: `Route "example"`,
				Body:  ListBody{"Upstream unavailable", "Retry pending"},
				Level: tt.level,
			})
			require.NoError(t, err)
			// MessageExternal includes these zero-valued response fields in v2 and v3.
			require.JSONEq(t, fmt.Sprintf(`{"id":0,"appid":0,"date":"0001-01-01T00:00:00Z","title":"Route \"example\"","message":"Upstream unavailable\nRetry pending","priority":%d}`, tt.priority), string(data))
		})
	}
}

func TestGotifyMarshalMessageMarkdown(t *testing.T) {
	client := &GotifyClient{ProviderBase: ProviderBase{Format: LogFormatMarkdown}}
	data, err := client.MarshalMessage(&LogMessage{
		Title: "Route unavailable",
		Body:  ListBody{"Upstream unavailable", "Retry pending"},
		Level: zerolog.WarnLevel,
	})
	require.NoError(t, err)
	require.JSONEq(t, `{
		"id":0,"appid":0,"date":"0001-01-01T00:00:00Z",
		"title":"Route unavailable",
		"message":"* Upstream unavailable\n* Retry pending\n",
		"priority":2,
		"extras":{"client::display":{"contentType":"text/markdown"}}
	}`, string(data))
}

func TestGotifyFmtError(t *testing.T) {
	client := &GotifyClient{}
	err := client.fmtError(strings.NewReader(`{"error":"Unauthorized","errorDescription":"invalid token","errorCode":401}`))
	require.EqualError(t, err, "Unauthorized: invalid token")

	t.Run("empty response", func(t *testing.T) {
		err := client.fmtError(strings.NewReader(""))
		require.ErrorIs(t, err, io.EOF)
		require.ErrorContains(t, err, "failed to decode err response:")
	})
	t.Run("malformed response", func(t *testing.T) {
		err := client.fmtError(strings.NewReader("not json"))
		require.ErrorContains(t, err, "failed to decode err response:")
		require.NotNil(t, errors.Unwrap(err))
	})
}
