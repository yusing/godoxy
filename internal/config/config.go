package config

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yusing/go-proxy/internal/api"
	"github.com/yusing/go-proxy/internal/autocert"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/entrypoint"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/net/http/server"
	"github.com/yusing/go-proxy/internal/notif"
	proxy "github.com/yusing/go-proxy/internal/route/provider"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils"
	F "github.com/yusing/go-proxy/internal/utils/functional"
	"github.com/yusing/go-proxy/internal/watcher"
	"github.com/yusing/go-proxy/internal/watcher/events"
)

type Config struct {
	value            *types.Config
	providers        F.Map[string, *proxy.Provider]
	autocertProvider *autocert.Provider
	entrypoint       *entrypoint.Entrypoint

	task *task.Task
}

var (
	instance   *Config
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

var Validate = types.Validate

func GetInstance() *Config {
	return instance
}

func newConfig() *Config {
	return &Config{
		value:      types.DefaultConfig(),
		providers:  F.NewMapOf[string, *proxy.Provider](),
		entrypoint: entrypoint.NewEntrypoint(),
		task:       task.RootTask("config", false),
	}
}

func Load() (*Config, gperr.Error) {
	if instance != nil {
		return instance, nil
	}
	instance = newConfig()
	cfgWatcher = watcher.NewConfigFileWatcher(common.ConfigFileName)
	return instance, instance.load()
}

func MatchDomains() []string {
	return instance.value.MatchDomains
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
		logging.Warn().Msg(cfgRenameWarn)
		return
	case events.ActionFileDeleted:
		logging.Warn().Msg(cfgDeleteWarn)
		return
	}

	if err := Reload(); err != nil {
		// recovered in event queue
		panic(err)
	}
}

func Reload() gperr.Error {
	// avoid race between config change and API reload request
	reloadMu.Lock()
	defer reloadMu.Unlock()

	newCfg := newConfig()
	err := newCfg.load()
	if err != nil {
		newCfg.task.Finish(err)
		return gperr.New("using last config").With(err)
	}

	// cancel all current subtasks -> wait
	// -> replace config -> start new subtasks
	instance.task.Finish("config changed")
	instance = newCfg
	instance.Start(StartAllServers)
	return nil
}

func (cfg *Config) Value() *types.Config {
	return instance.value
}

func (cfg *Config) Reload() gperr.Error {
	return Reload()
}

func (cfg *Config) AutoCertProvider() *autocert.Provider {
	return instance.autocertProvider
}

func (cfg *Config) Task() *task.Task {
	return cfg.task
}

func (cfg *Config) Context() context.Context {
	return cfg.task.Context()
}

func (cfg *Config) Start(opts ...*StartServersOptions) {
	cfg.StartAutoCert()
	cfg.StartProxyProviders()
	cfg.StartServers(opts...)
}

func (cfg *Config) StartAutoCert() {
	autocert := cfg.autocertProvider
	if autocert == nil {
		logging.Info().Msg("autocert not configured")
		return
	}

	if err := autocert.Setup(); err != nil {
		gperr.LogFatal("autocert setup error", err)
	} else {
		autocert.ScheduleRenewal(cfg.task)
	}
}

func (cfg *Config) StartProxyProviders() {
	errs := cfg.providers.CollectErrors(
		func(_ string, p *proxy.Provider) error {
			return p.Start(cfg.task)
		})

	if err := gperr.Join(errs...); err != nil {
		gperr.LogError("route provider errors", err)
	}
}

type StartServersOptions struct {
	Proxy, API bool
}

var StartAllServers = &StartServersOptions{true, true}

func (cfg *Config) StartServers(opts ...*StartServersOptions) {
	if len(opts) == 0 {
		opts = append(opts, &StartServersOptions{})
	}
	opt := opts[0]
	if opt.Proxy {
		server.StartServer(cfg.task, server.Options{
			Name:         "proxy",
			CertProvider: cfg.AutoCertProvider(),
			HTTPAddr:     common.ProxyHTTPAddr,
			HTTPSAddr:    common.ProxyHTTPSAddr,
			Handler:      cfg.entrypoint,
		})
	}
	if opt.API {
		server.StartServer(cfg.task, server.Options{
			Name:         "api",
			CertProvider: cfg.AutoCertProvider(),
			HTTPAddr:     common.APIHTTPAddr,
			Handler:      api.NewHandler(cfg),
		})
	}
}

func (cfg *Config) load() gperr.Error {
	const errMsg = "config load error"

	data, err := os.ReadFile(common.ConfigPath)
	if err != nil {
		gperr.LogFatal(errMsg, err)
	}

	model := types.DefaultConfig()
	if err := utils.DeserializeYAML(data, model); err != nil {
		gperr.LogFatal(errMsg, err)
	}

	// errors are non fatal below
	errs := gperr.NewBuilder(errMsg)
	errs.Add(cfg.entrypoint.SetMiddlewares(model.Entrypoint.Middlewares))
	errs.Add(cfg.entrypoint.SetAccessLogger(cfg.task, model.Entrypoint.AccessLog))
	cfg.initNotification(model.Providers.Notification)
	errs.Add(cfg.initAutoCert(model.AutoCert))
	errs.Add(cfg.loadRouteProviders(&model.Providers))

	cfg.value = model
	for i, domain := range model.MatchDomains {
		if !strings.HasPrefix(domain, ".") {
			model.MatchDomains[i] = "." + domain
		}
	}
	cfg.entrypoint.SetFindRouteDomains(model.MatchDomains)

	return errs.Error()
}

func (cfg *Config) initNotification(notifCfg []notif.NotificationConfig) {
	if len(notifCfg) == 0 {
		return
	}
	dispatcher := notif.StartNotifDispatcher(cfg.task)
	for _, notifier := range notifCfg {
		dispatcher.RegisterProvider(&notifier)
	}
}

func (cfg *Config) initAutoCert(autocertCfg *autocert.AutocertConfig) (err gperr.Error) {
	if cfg.autocertProvider != nil {
		return
	}

	cfg.autocertProvider, err = autocertCfg.GetProvider()
	return
}

func (cfg *Config) loadRouteProviders(providers *config.Providers) gperr.Error {

	lenLongestName := 0
	for _, filename := range providers.Files {
		p, err := proxy.NewFileProvider(filename)
		if err != nil {
			errs.Add(E.PrependSubject(filename, err))
			continue
		}
		cfg.providers.Store(p.String(), p)
		if len(p.String()) > lenLongestName {
			lenLongestName = len(p.String())
		}
	}
	for name, dockerHost := range providers.Docker {
		p, err := proxy.NewDockerProvider(name, dockerHost)
		if err != nil {
			errs.Add(E.PrependSubject(name, err))
			continue
		}
		cfg.providers.Store(p.String(), p)
		if len(p.String()) > lenLongestName {
			lenLongestName = len(p.String())
		}
	}
	cfg.providers.RangeAllParallel(func(_ string, p *proxy.Provider) {
		if err := p.LoadRoutes(); err != nil {
			errs.Add(err.Subject(p.String()))
		}
		results.Addf("%-"+strconv.Itoa(lenLongestName)+"s %d routes", p.String(), p.NumRoutes())
	})
	logging.Info().Msg(results.String())
	return errs.Error()
}
