package config

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/internal/api"
	"github.com/yusing/go-proxy/internal/autocert"
	"github.com/yusing/go-proxy/internal/common"
	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/entrypoint"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/net/gphttp/server"
	"github.com/yusing/go-proxy/internal/notif"
	"github.com/yusing/go-proxy/internal/proxmox"
	proxy "github.com/yusing/go-proxy/internal/route/provider"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils"
	F "github.com/yusing/go-proxy/internal/utils/functional"
	"github.com/yusing/go-proxy/internal/watcher"
	"github.com/yusing/go-proxy/internal/watcher/events"
)

type Config struct {
	value            *config.Config
	providers        F.Map[string, *proxy.Provider]
	autocertProvider *autocert.Provider
	entrypoint       *entrypoint.Entrypoint

	task *task.Task
}

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

var Validate = config.Validate

var ErrProviderNameConflict = gperr.New("provider name conflict")

func newConfig() *Config {
	return &Config{
		value:      config.DefaultConfig(),
		providers:  F.NewMapOf[string, *proxy.Provider](),
		entrypoint: entrypoint.NewEntrypoint(),
		task:       task.RootTask("config", false),
	}
}

func Load() (*Config, gperr.Error) {
	if config.HasInstance() {
		panic(errors.New("config already loaded"))
	}
	cfg := newConfig()
	config.SetInstance(cfg)
	cfgWatcher = watcher.NewConfigFileWatcher(common.ConfigFileName)
	return cfg, cfg.load()
}

func MatchDomains() []string {
	return config.GetInstance().Value().MatchDomains
}

func WatchChanges() {
	t := task.RootTask("config_watcher", true)
	eventQueue := events.NewEventQueue(
		t,
		configEventFlushInterval,
		OnConfigChange,
		onReloadError,
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

func onReloadError(err gperr.Error) {
	logging.Error().Msgf("config reload error: %s", err)
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
	config.GetInstance().(*Config).Task().Finish("config changed")
	newCfg.Start(StartAllServers)
	config.SetInstance(newCfg)
	return nil
}

func (cfg *Config) Value() *config.Config {
	return cfg.value
}

func (cfg *Config) Reload() gperr.Error {
	return Reload()
}

// AutoCertProvider returns the autocert provider.
//
// If the autocert provider is not configured, it returns nil.
func (cfg *Config) AutoCertProvider() *autocert.Provider {
	return cfg.autocertProvider
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
	data, err := os.ReadFile(common.ConfigPath)
	if err != nil {
		gperr.LogFatal("error reading config", err)
	}

	model := config.DefaultConfig()
	if err := utils.UnmarshalValidateYAML(data, model); err != nil {
		gperr.LogFatal("error unmarshalling config", err)
	}

	// errors are non fatal below
	errs := gperr.NewBuilder()
	errs.Add(cfg.entrypoint.SetMiddlewares(model.Entrypoint.Middlewares))
	errs.Add(cfg.entrypoint.SetAccessLogger(cfg.task, model.Entrypoint.AccessLog))
	cfg.initNotification(model.Providers.Notification)
	errs.Add(cfg.initProxmox(model.Providers.Proxmox))
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

func (cfg *Config) initProxmox(proxmoxCfgs []proxmox.Config) (err gperr.Error) {
	errs := gperr.NewBuilder("proxmox config errors")
	for _, proxmoxCfg := range proxmoxCfgs {
		if err := proxmoxCfg.Init(); err != nil {
			errs.Add(err.Subject(proxmoxCfg.URL))
		} else {
			proxmox.Clients.Add(proxmoxCfg.Client())
		}
	}
	return errs.Error()
}

func (cfg *Config) initAutoCert(autocertCfg *autocert.AutocertConfig) (err gperr.Error) {
	if cfg.autocertProvider != nil {
		return
	}

	cfg.autocertProvider, err = autocertCfg.GetProvider()
	return
}

func (cfg *Config) errIfExists(p *proxy.Provider) gperr.Error {
	if conflict, ok := cfg.providers.Load(p.String()); ok {
		return ErrProviderNameConflict.
			Subject(p.String()).
			Withf("one is %q", conflict.Type()).
			Withf("the other is %q", p.Type())
	}
	return nil
}

func (cfg *Config) initAgents(agentCfgs []*agent.AgentConfig) gperr.Error {
	var wg sync.WaitGroup

	errs := gperr.NewBuilderWithConcurrency()
	wg.Add(len(agentCfgs))
	for _, agentCfg := range agentCfgs {
		go func(agentCfg *agent.AgentConfig) {
			defer wg.Done()
			if err := agentCfg.Init(cfg.task.Context()); err != nil {
				errs.Add(err.Subject(agentCfg.String()))
			} else {
				agent.Agents.Add(agentCfg)
			}
		}(agentCfg)
	}
	wg.Wait()
	return errs.Error()
}

func (cfg *Config) loadRouteProviders(providers *config.Providers) gperr.Error {
	errs := gperr.NewBuilder("route provider errors")
	results := gperr.NewBuilder("loaded route providers")

	agent.Agents.Clear()

	n := len(providers.Agents) + len(providers.Docker) + len(providers.Files)
	if n == 0 {
		return nil
	}

	routeProviders := make([]*proxy.Provider, 0, n)

	errs.Add(cfg.initAgents(providers.Agents))

	for _, a := range providers.Agents {
		if !a.IsInitialized() { // failed to initialize
			continue
		}
		agent.Agents.Add(a)
		routeProviders = append(routeProviders, proxy.NewAgentProvider(a))
	}
	for _, filename := range providers.Files {
		routeProviders = append(routeProviders, proxy.NewFileProvider(filename))
	}
	for name, dockerHost := range providers.Docker {
		routeProviders = append(routeProviders, proxy.NewDockerProvider(name, dockerHost))
	}

	// check if all providers are unique (should not happen but just in case)
	for _, p := range routeProviders {
		if err := cfg.errIfExists(p); err != nil {
			errs.Add(err)
			continue
		}
		cfg.providers.Store(p.String(), p)
	}

	lenLongestName := 0
	cfg.providers.RangeAll(func(k string, _ *proxy.Provider) {
		if len(k) > lenLongestName {
			lenLongestName = len(k)
		}
	})
	errs.EnableConcurrency()
	results.EnableConcurrency()
	cfg.providers.RangeAllParallel(func(_ string, p *proxy.Provider) {
		if err := p.LoadRoutes(); err != nil {
			errs.Add(err.Subject(p.String()))
		}
		results.Addf("%-"+strconv.Itoa(lenLongestName)+"s %d routes", p.String(), p.NumRoutes())
	})
	logging.Info().Msg(results.String())
	return errs.Error()
}
