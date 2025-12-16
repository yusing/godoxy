package config

import (
	"errors"
	"fmt"
	"io/fs"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/common"
	config "github.com/yusing/godoxy/internal/config/types"
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

var nilState *state

func Load() error {
	if HasState() {
		panic(errors.New("config already loaded"))
	}
	state := NewState()
	config.WorkingState.Store(state)
	defer config.WorkingState.Store(nilState)

	cfgWatcher = watcher.NewConfigFileWatcher(common.ConfigFileName)

	initErr := state.InitFromFile(common.ConfigPath)
	if errors.Is(initErr, fs.ErrNotExist) {
		// log only
		log.Warn().Msg("config file not found, using default config")
		initErr = nil
	}
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
	defer config.WorkingState.Store(nilState)

	err := newState.InitFromFile(common.ConfigPath)
	if err != nil {
		newState.Task().FinishAndWait(err)
		logNotifyError("reload", err)
		return gperr.New(ansi.Warning("using last config")).With(err)
	}

	// flush temporary log
	newState.FlushTmpLog()

	// cancel all current subtasks -> wait
	// -> replace config -> start new subtasks
	GetState().Task().FinishAndWait(config.ErrConfigChanged)
	SetState(newState)

	if err := newState.StartProviders(); err != nil {
		gperr.LogWarn("start providers error", err)
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
