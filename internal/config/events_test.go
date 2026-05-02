package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/common"
	configtypes "github.com/yusing/godoxy/internal/config/types"
	watcherEvents "github.com/yusing/godoxy/internal/watcher/events"
)

func TestOnConfigChangeManagedConfigReloadSuppression(t *testing.T) {
	previousReloadConfig := reloadConfig
	reloadCalls := 0

	reloadConfig = func() error {
		reloadCalls++
		return nil
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
