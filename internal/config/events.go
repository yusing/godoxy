package config

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/notif"
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

func Load() error {
	if HasState() {
		panic(errors.New("config already loaded"))
	}
	state := NewState()
	cfgWatcher = watcher.NewConfigFileWatcher(common.ConfigFileName)

	err := errors.Join(state.InitFromFileOrExit(common.ConfigPath), state.StartProviders())
	if err != nil {
		notifyError("init", err)
	}
	SetState(state)
	return nil
}

func notifyError(action string, err error) {
	notif.Notify(&notif.LogMessage{
		Level: zerolog.ErrorLevel,
		Title: fmt.Sprintf("Config %s Error", action),
		Body:  notif.ErrorBody(err),
	})
}

func Reload() gperr.Error {
	// avoid race between config change and API reload request
	reloadMu.Lock()
	defer reloadMu.Unlock()

	newState := NewState()
	err := newState.InitFromFileOrExit(common.ConfigPath)
	if err != nil {
		newState.Task().FinishAndWait(err)
		notifyError("reload", err)
		return gperr.New(ansi.Warning("using last config")).With(err)
	}

	// cancel all current subtasks -> wait
	// -> replace config -> start new subtasks
	GetState().Task().FinishAndWait("config changed")
	SetState(newState)

	if err := newState.StartProviders(); err != nil {
		gperr.LogWarn("start providers error", err)
		notifyError("start providers", err)
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
			gperr.LogError("config reload error", err)
		},
	)
	eventQueue.Start(cfgWatcher.Events(t.Context()))
}

func OnConfigChange(ev []events.Event) {
	// no matter how many events during the interval
	// just reload once and check the last event
	switch ev[len(ev)-1].Action {
	case events.ActionFileRenamed:
		log.Warn().Msg(cfgRenameWarn)
		return
	case events.ActionFileDeleted:
		log.Warn().Msg(cfgDeleteWarn)
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
