package config

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	agentPkg "github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/internal/api"
	autocert "github.com/yusing/go-proxy/internal/autocert"
	"github.com/yusing/go-proxy/internal/common"
	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/entrypoint"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/maxmind"
	"github.com/yusing/go-proxy/internal/net/gphttp/server"
	"github.com/yusing/go-proxy/internal/notif"
	"github.com/yusing/go-proxy/internal/proxmox"
	proxy "github.com/yusing/go-proxy/internal/route/provider"
	"github.com/yusing/go-proxy/internal/serialization"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils/strutils/ansi"
	"github.com/yusing/go-proxy/internal/watcher"
	"github.com/yusing/go-proxy/internal/watcher/events"
)

type Config struct {
	value            *config.Config
	providers        *xsync.Map[string, *proxy.Provider]
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

func newConfig() *Config {
	return &Config{
		value:      config.DefaultConfig(),
		providers:  xsync.NewMap[string, *proxy.Provider](),
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

func Reload() gperr.Error {
	// avoid race between config change and API reload request
	reloadMu.Lock()
	defer reloadMu.Unlock()

	newCfg := newConfig()
	err := newCfg.load()
	if err != nil {
		newCfg.task.FinishAndWait(err)
		return gperr.New(ansi.Warning("using last config")).With(err)
	}

	// cancel all current subtasks -> wait
	// -> replace config -> start new subtasks
	config.GetInstance().(*Config).Task().FinishAndWait("config changed")
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
		log.Info().Msg("autocert not configured")
		return
	}

	if err := autocert.Setup(); err != nil {
		gperr.LogFatal("autocert setup error", err)
	} else {
		autocert.ScheduleRenewal(cfg.task)
	}
}

func (cfg *Config) StartProxyProviders() {
	var wg sync.WaitGroup

	errs := gperr.NewBuilderWithConcurrency()
	for _, p := range cfg.providers.Range {
		wg.Go(func() {
			if err := p.Start(cfg.task); err != nil {
				errs.Add(err.Subject(p.String()))
			}
		})
	}
	wg.Wait()

	if err := errs.Error(); err != nil {
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
			ACL:          cfg.value.ACL,
		})
	}
	if opt.API {
		server.StartServer(cfg.task, server.Options{
			Name:         "api",
			CertProvider: cfg.AutoCertProvider(),
			HTTPAddr:     common.APIHTTPAddr,
			Handler:      api.NewHandler(),
		})
	}
}

func (cfg *Config) load() gperr.Error {
	const errMsg = "config load error"

	data, err := os.ReadFile(common.ConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Warn().Msg("config file not found, using default config")
			cfg.value = config.DefaultConfig()
			return nil
		} else {
			gperr.LogFatal(errMsg, err)
		}
	}

	model := config.DefaultConfig()
	if err := serialization.UnmarshalValidateYAML(data, model); err != nil {
		gperr.LogFatal(errMsg, err)
	}

	// errors are non fatal below
	errs := gperr.NewBuilder(errMsg)
	errs.Add(cfg.entrypoint.SetMiddlewares(model.Entrypoint.Middlewares))
	errs.Add(cfg.entrypoint.SetAccessLogger(cfg.task, model.Entrypoint.AccessLog))
	errs.Add(cfg.initMaxMind(model.Providers.MaxMind))
	cfg.initNotification(model.Providers.Notification)
	errs.Add(cfg.initAutoCert(model.AutoCert))
	errs.Add(cfg.initProxmox(model.Providers.Proxmox))
	errs.Add(cfg.loadRouteProviders(&model.Providers))

	cfg.value = model
	for i, domain := range model.MatchDomains {
		if !strings.HasPrefix(domain, ".") {
			model.MatchDomains[i] = "." + domain
		}
	}
	cfg.entrypoint.SetFindRouteDomains(model.MatchDomains)
	if model.ACL.Valid() {
		err := model.ACL.Start(cfg.task)
		if err != nil {
			errs.Add(err)
		}
	}

	if errs.HasError() {
		notif.Notify(&notif.LogMessage{
			Level: zerolog.ErrorLevel,
			Title: "Config Reload Error",
			Body:  notif.ErrorBody{Error: errs.Error()},
		})
		return errs.Error()
	}
	return nil
}

func (cfg *Config) initMaxMind(maxmindCfg *maxmind.Config) gperr.Error {
	if maxmindCfg != nil {
		return maxmind.SetInstance(cfg.task, maxmindCfg)
	}
	return nil
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

func (cfg *Config) initAutoCert(autocertCfg *autocert.Config) gperr.Error {
	if cfg.autocertProvider != nil {
		return nil
	}

	if autocertCfg == nil {
		autocertCfg = new(autocert.Config)
	}

	user, legoCfg, err := autocertCfg.GetLegoConfig()
	if err != nil {
		return err
	}

	cfg.autocertProvider = autocert.NewProvider(autocertCfg, user, legoCfg)
	return nil
}

func (cfg *Config) initProxmox(proxmoxCfg []proxmox.Config) gperr.Error {
	proxmox.Clients.Clear()
	errs := gperr.NewBuilder()
	for _, cfg := range proxmoxCfg {
		if err := cfg.Init(); err != nil {
			errs.Add(err.Subject(cfg.URL))
		}
	}
	return errs.Error()
}

func (cfg *Config) storeProvider(p *proxy.Provider) {
	cfg.providers.Store(p.String(), p)
}

func (cfg *Config) loadRouteProviders(providers *config.Providers) gperr.Error {
	errs := gperr.NewBuilderWithConcurrency("route provider errors")
	results := gperr.NewBuilder("loaded route providers")

	agentPkg.RemoveAllAgents()

	numProviders := len(providers.Agents) + len(providers.Files) + len(providers.Docker)
	providersCh := make(chan *proxy.Provider, numProviders)

	// start providers concurrently
	var providersConsumer sync.WaitGroup
	providersConsumer.Go(func() {
		for p := range providersCh {
			if actual, loaded := cfg.providers.LoadOrStore(p.String(), p); loaded {
				errs.Add(gperr.Errorf("provider %s already exists, first: %s, second: %s", p.String(), actual.GetType(), p.GetType()))
				continue
			}
			cfg.storeProvider(p)
		}
	})

	var providersProducer sync.WaitGroup
	for _, agent := range providers.Agents {
		providersProducer.Go(func() {
			if err := agent.Start(cfg.task.Context()); err != nil {
				errs.Add(gperr.PrependSubject(agent.String(), err))
				return
			}
			agentPkg.AddAgent(agent)
			p := proxy.NewAgentProvider(agent)
			providersCh <- p
		})
	}

	for _, filename := range providers.Files {
		providersProducer.Go(func() {
			p, err := proxy.NewFileProvider(filename)
			if err != nil {
				errs.Add(gperr.PrependSubject(filename, err))
			} else {
				providersCh <- p
			}
		})
	}

	for name, dockerHost := range providers.Docker {
		providersProducer.Go(func() {
			providersCh <- proxy.NewDockerProvider(name, dockerHost)
		})
	}

	providersProducer.Wait()

	close(providersCh)
	providersConsumer.Wait()

	lenLongestName := 0
	for k := range cfg.providers.Range {
		if len(k) > lenLongestName {
			lenLongestName = len(k)
		}
	}

	results.EnableConcurrency()

	// load routes concurrently
	var providersLoader sync.WaitGroup
	for _, p := range cfg.providers.Range {
		providersLoader.Go(func() {
			if err := p.LoadRoutes(); err != nil {
				errs.Add(err.Subject(p.String()))
			}
			results.Addf("%-"+strconv.Itoa(lenLongestName)+"s %d routes", p.String(), p.NumRoutes())
		})
	}
	providersLoader.Wait()

	log.Info().Msg(results.String())
	return errs.Error()
}
