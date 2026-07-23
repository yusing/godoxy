package config

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/common"
	configtypes "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/notif"
	watcherEvents "github.com/yusing/godoxy/internal/watcher/events"
)

func TestOnConfigChangeManagedConfigReloadSuppression(t *testing.T) {
	previousReloadConfig := reloadConfig
	reloadCalls := 0

	reloadConfig = func() configtypes.ReloadResult {
		reloadCalls++
		return configtypes.ReloadResult{}
	}
	t.Cleanup(func() {
		reloadConfig = previousReloadConfig
		configtypes.ClearConfigReloadSuppression()
	})

	newEvent := func(actorName string, action watcherEvents.Action) watcherEvents.Event {
		return watcherEvents.Event{
			ActorName: actorName,
			Action:    action,
		}
	}

	t.Run("suppresses one matching write batch and consumes suppression", func(t *testing.T) {
		configtypes.ClearConfigReloadSuppression()
		reloadCalls = 0

		configtypes.SuppressNextConfigReloadUntil(time.Now().Add(time.Minute))
		OnConfigChange([]watcherEvents.Event{newEvent(common.ConfigFileName, watcherEvents.ActionFileWritten)})
		require.Equal(t, 0, reloadCalls)

		OnConfigChange([]watcherEvents.Event{newEvent(common.ConfigFileName, watcherEvents.ActionFileWritten)})
		require.Equal(t, 1, reloadCalls)
	})

	t.Run("suppresses one matching create batch and consumes suppression", func(t *testing.T) {
		configtypes.ClearConfigReloadSuppression()
		reloadCalls = 0

		configtypes.SuppressNextConfigReloadUntil(time.Now().Add(time.Minute))
		OnConfigChange([]watcherEvents.Event{newEvent(common.ConfigFileName, watcherEvents.ActionFileCreated)})
		require.Equal(t, 0, reloadCalls)

		OnConfigChange([]watcherEvents.Event{newEvent(common.ConfigFileName, watcherEvents.ActionFileCreated)})
		require.Equal(t, 1, reloadCalls)
	})

	t.Run("does not suppress unrelated file events", func(t *testing.T) {
		configtypes.ClearConfigReloadSuppression()
		reloadCalls = 0

		configtypes.SuppressNextConfigReloadUntil(time.Now().Add(time.Minute))
		OnConfigChange([]watcherEvents.Event{newEvent("routes.yml", watcherEvents.ActionFileWritten)})
		require.Equal(t, 1, reloadCalls)

		OnConfigChange([]watcherEvents.Event{newEvent(common.ConfigFileName, watcherEvents.ActionFileWritten)})
		require.Equal(t, 1, reloadCalls)
	})

	t.Run("expired suppression does not block later reloads", func(t *testing.T) {
		configtypes.ClearConfigReloadSuppression()
		reloadCalls = 0

		configtypes.SuppressNextConfigReloadUntil(time.Now().Add(-time.Second))

		OnConfigChange([]watcherEvents.Event{newEvent(common.ConfigFileName, watcherEvents.ActionFileWritten)})
		require.Equal(t, 1, reloadCalls)
	})
}

func TestNotificationDispatcherActivationLifecycle(t *testing.T) {
	oldHits := make(chan string, 4)
	oldServer := newNotificationTestServer(t, oldHits)
	newHits := make(chan string, 4)
	newServer := newNotificationTestServer(t, newHits)

	oldState := newNotificationTestState(t, oldServer.URL)
	defer oldState.Task().FinishAndWait(nil)
	require.NoError(t, oldState.initNotification())

	newState := newNotificationTestState(t, newServer.URL)
	defer newState.Task().FinishAndWait(nil)
	require.NoError(t, newState.initNotification())

	notif.FromCtx(oldState.Context()).Notify(&notif.LogMessage{Title: "before-activation", Body: notif.MessageBody("before-activation")})
	require.Equal(t, "before-activation", receiveNotification(t, oldHits))
	requireNoNotification(t, newHits)

	newState.Task().FinishAndWait(assertErr("failed reload"))
	notif.FromCtx(oldState.Context()).Notify(&notif.LogMessage{Title: "after-failed-reload", Body: notif.MessageBody("after-failed-reload")})
	require.Equal(t, "after-failed-reload", receiveNotification(t, oldHits))
	requireNoNotification(t, newHits)

	nextState := newNotificationTestState(t, newServer.URL)
	defer nextState.Task().FinishAndWait(nil)
	require.NoError(t, nextState.initNotification())
	oldState.Task().FinishAndWait(configtypes.ErrConfigChanged)

	notif.FromCtx(nextState.Context()).Notify(&notif.LogMessage{Title: "after-activation", Body: notif.MessageBody("after-activation")})
	require.Equal(t, "after-activation", receiveNotification(t, newHits))
	requireNoNotification(t, oldHits)
}

type assertErr string

func (err assertErr) Error() string {
	return string(err)
}

func newNotificationTestState(t *testing.T, url string) *state {
	t.Helper()
	state := NewState()
	state.Providers.Notification = []*notif.NotificationConfig{
		{
			ProviderName: notif.ProviderWebhook,
			Provider: &notif.Webhook{
				ProviderBase: notif.ProviderBase{
					Name:   "test",
					URL:    url,
					Format: notif.LogFormatPlain,
				},
				Payload:  "$message",
				Method:   http.MethodPost,
				MIMEType: notif.MimeTypeText,
			},
		},
	}
	return state
}

func newNotificationTestServer(t *testing.T, hits chan<- string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		hits <- string(body)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	return server
}

func receiveNotification(t *testing.T, hits <-chan string) string {
	t.Helper()
	select {
	case hit := <-hits:
		return hit
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notification")
		return ""
	}
}

func requireNoNotification(t *testing.T, hits <-chan string) {
	t.Helper()
	select {
	case hit := <-hits:
		t.Fatalf("unexpected notification: %s", hit)
	case <-time.After(50 * time.Millisecond):
	}
}
