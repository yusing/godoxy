package notif

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiscordJoinedErrorBody(t *testing.T) {
	webhook := &Webhook{Template: "discord"}
	body, err := webhook.MarshalMessage(&LogMessage{
		Title: "Configuration lifecycle degraded",
		Body:  ErrorBody(errors.Join(errors.New("DNS unavailable"), errors.New("provider failed"))),
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"embeds":[{"title":"Configuration lifecycle degraded","fields":[{"name":"Error","value":"DNS unavailable\nprovider failed"}],"color":"0"}]}`, string(body))
}

