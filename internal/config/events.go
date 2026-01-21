package config

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/yusing/godoxy/internal/common"
	config "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/notif"
	"github.com/yusing/godoxy/internal/route/routes"
	"github.com/yusing/godoxy/internal/watcher"
	"github.com/yusing/godoxy/internal/watcher/events"
	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/server"
	"github.com/yusing/goutils/strings/ansi"
	"github.com/yusing/goutils/task"
)

var (
	cfgWatcher watcher.Watcher
	reloadMu   sync.Mutex
)

const configEventFlushInterval = 500 * time.Millisecond

const (
	cfgRenameWarn = `Config file renamed, not reloading.
Make sure you rename it back before next time you start.`
	cfgDeleteWarn = `Config file deleted, not reloading.
You may run "ls-config" to show or dump the current config.`
)

func logNotifyError(action string, err error) {
	gperr.LogError("config "+action+" error", err)
	notif.Notify(&notif.LogMessage{
		Level: zerolog.ErrorLevel,
		Title: fmt.Sprintf("Config %s error", action),
		Body:  notif.ErrorBody(err),
	})
}

func logNotifyWarn(action string, err error) {
	gperr.LogWarn("config "+action+" error", err)
	notif.Notify(&notif.LogMessage{
		Level: zerolog.WarnLevel,
		Title: fmt.Sprintf("Config %s warning", action),
		Body:  notif.ErrorBody(err),
	})
}

func Load() error {
	if HasState() {
		panic(errors.New("config already loaded"))
	}
	state := NewState()
	config.WorkingState.Store(state)

	cfgWatcher = watcher.NewConfigFileWatcher(common.ConfigFileName)

	// disable pool logging temporary since we already have pretty logging
	routes.HTTP.DisableLog(true)
	routes.Stream.DisableLog(true)

	defer func() {
		routes.HTTP.DisableLog(false)
		routes.Stream.DisableLog(false)
	}()

	initErr := state.InitFromFile(common.ConfigPath)
	err := errors.Join(initErr, state.StartProviders())
	if err != nil {
		logNotifyError("init", err)
	}
	SetState(state)

	// flush temporary log
	state.FlushTmpLog()
	return nil
}

func Reload() gperr.Error {
	// avoid race between config change and API reload request
	reloadMu.Lock()
	defer reloadMu.Unlock()

	newState := NewState()
	config.WorkingState.Store(newState)

	err := newState.InitFromFile(common.ConfigPath)
	if err != nil {
		newState.Task().FinishAndWait(err)
		config.WorkingState.Store(GetState())
		return gperr.Wrap(err, ansi.Warning("using last config"))
	}

	// flush temporary log
	newState.FlushTmpLog()

	// cancel all current subtasks -> wait
	// -> replace config -> start new subtasks
	GetState().Task().FinishAndWait(config.ErrConfigChanged)
	SetState(newState)

	if err := newState.StartProviders(); err != nil {
		logNotifyError("start providers", err)
		return nil // continue
	}
	StartProxyServers()
	return nil
}

func WatchChanges() {
	t := task.RootTask("config_watcher", true)
	eventQueue := events.NewEventQueue(
		t,
		configEventFlushInterval,
		OnConfigChange,
		func(err gperr.Error) {
			logNotifyError("reload", err)
		},
	)
	eventQueue.Start(cfgWatcher.Events(t.Context()))
}

func OnConfigChange(ev []events.Event) {
	// no matter how many events during the interval
	// just reload once and check the last event
	switch ev[len(ev)-1].Action {
	case events.ActionFileRenamed:
		logNotifyWarn("rename", errors.New(cfgRenameWarn))
		return
	case events.ActionFileDeleted:
		logNotifyWarn("delete", errors.New(cfgDeleteWarn))
		return
	}

	if err := Reload(); err != nil {
		// recovered in event queue
		panic(err)
	}
}

func StartProxyServers() {
	cfg := GetState()
	server.StartServer(cfg.Task(), server.Options{
		Name:                 "proxy",
		CertProvider:         cfg.AutoCertProvider(),
		HTTPAddr:             common.ProxyHTTPAddr,
		HTTPSAddr:            common.ProxyHTTPSAddr,
		Handler:              cfg.EntrypointHandler(),
		ACL:                  cfg.Value().ACL,
		SupportProxyProtocol: cfg.Value().Entrypoint.SupportProxyProtocol,
	})
}
