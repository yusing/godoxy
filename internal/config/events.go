package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/common"
	configtypes "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/notif"
	"github.com/yusing/godoxy/internal/watcher"
	watcherEvents "github.com/yusing/godoxy/internal/watcher/events"
	"github.com/yusing/goutils/eventqueue"
	"github.com/yusing/goutils/events"
	"github.com/yusing/goutils/task"
)

var (
	cfgWatcher   watcher.Watcher
	reloadConfig = Reload
)

const configEventFlushInterval = 500 * time.Millisecond

var (
	errCfgRenameWarn = errors.New("config file renamed, not reloading; Make sure you rename it back before next time you start")
	errCfgDeleteWarn = errors.New(`config file deleted, not reloading; You may run "ls-config" to show or dump the current config`)
)

func logNotify(level zerolog.Level, action string, err error) {
	ctx := task.RootContext()
	if state := defaultRuntimeManager.RuntimeState(); state != nil {
		ctx = state.Context()
	}

	log.WithLevel(level).Err(err).Msg("config " + action)
	notif.FromCtx(ctx).Notify(&notif.LogMessage{
		Level: level,
		Title: fmt.Sprintf("Config %s", action),
		Body:  notif.ErrorBody(err),
	})
	eventLevel := events.LevelWarn
	if level >= zerolog.ErrorLevel {
		eventLevel = events.LevelError
	}
	defaultRuntimeManager.history.Add(events.NewEvent(eventLevel, "config", action, err))
}

func WatchChanges() {
	cfgWatcher = watcher.NewConfigFileWatcher(common.ConfigFileName)
	opts := eventqueue.Options[watcherEvents.Event]{
		FlushInterval: configEventFlushInterval,
		OnFlush:       OnConfigChange,
		OnError: func(err error) {
			logNotify(zerolog.ErrorLevel, "reload", err)
		},
		Debug: common.IsDebug,
	}
	t := task.RootTask("config_watcher", true)
	eventQueue := eventqueue.New(t, opts)
	stream := cfgWatcher.Watch(t)
	eventQueue.Start(stream.Events, stream.Errors)
}

func OnConfigChange(ev []watcherEvents.Event) {
	if len(ev) == 0 {
		return
	}
	if configtypes.ConsumeSuppressedConfigReload(ev, common.ConfigFileName) {
		return
	}

	// No matter how many events arrive during the interval, reload once and
	// classify the final filesystem action.
	switch ev[len(ev)-1].Action {
	case watcherEvents.ActionFileRenamed:
		logNotify(zerolog.WarnLevel, "rename", errCfgRenameWarn)
		return
	case watcherEvents.ActionFileDeleted:
		logNotify(zerolog.WarnLevel, "delete", errCfgDeleteWarn)
		return
	}

	reloadConfig()
}
