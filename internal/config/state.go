package config

import (
	"context"
	"fmt"
	"iter"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/internal/acl"
	"github.com/yusing/godoxy/internal/autocert"
	"github.com/yusing/godoxy/internal/common"
	config "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/entrypoint"
	homepage "github.com/yusing/godoxy/internal/homepage/types"
	"github.com/yusing/godoxy/internal/maxmind"
	"github.com/yusing/godoxy/internal/notif"
	route "github.com/yusing/godoxy/internal/route/provider"
	"github.com/yusing/godoxy/internal/serialization"
	"github.com/yusing/godoxy/internal/types"
	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/server"
	"github.com/yusing/goutils/task"
)

type state struct {
	config.Config

	providers        *xsync.Map[string, types.RouteProvider]
	autocertProvider *autocert.Provider
	entrypoint       entrypoint.Entrypoint

	task *task.Task
}

func NewState() config.State {
	return &state{
		providers:  xsync.NewMap[string, types.RouteProvider](),
		entrypoint: entrypoint.NewEntrypoint(),
		task:       task.RootTask("config", false),
	}
}

var stateMu sync.RWMutex

func GetState() config.State {
	return config.ActiveState.Load()
}

func SetState(state config.State) {
	stateMu.Lock()
	defer stateMu.Unlock()

	cfg := state.Value()
	config.ActiveConfig.Store(cfg)
	config.ActiveState.Store(state)
	acl.ActiveConfig.Store(cfg.ACL)
	entrypoint.ActiveConfig.Store(&cfg.Entrypoint)
	homepage.ActiveConfig.Store(&cfg.Homepage)
	autocert.ActiveProvider.Store(state.AutoCertProvider().(*autocert.Provider))
}

func HasState() bool {
	return config.ActiveState.Load() != nil
}

func Value() *config.Config {
	return config.ActiveConfig.Load()
}

func (state *state) InitFromFileOrExit(filename string) error {
	data, err := os.ReadFile(common.ConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Warn().Msg("config file not found, using default config")
			state.Config = *config.DefaultConfig()
			return nil
		} else {
			gperr.LogFatal("config init error", err)
		}
	}
	return state.Init(data)
}

func (state *state) Init(data []byte) error {
	err := serialization.UnmarshalValidateYAML(data, &state.Config)
	if err != nil {
		return err
	}

	g := gperr.NewGroup("config load error")
	g.Go(state.initMaxMind)
	g.Go(state.initProxmox)
	g.Go(state.loadRouteProviders)
	g.Go(state.initAutoCert)

	errs := g.Wait()
	// these won't benefit from running on goroutines
	errs.Add(state.initNotification())
	errs.Add(state.initAccessLogger())
	errs.Add(state.initEntrypoint())
	// this must be run after loadRouteProviders
	errs.Add(state.startRouteProviders())
	return errs.Error()
}

func (state *state) Task() *task.Task {
	return state.task
}

func (state *state) Context() context.Context {
	return state.task.Context()
}

func (state *state) Value() *config.Config {
	return &state.Config
}

func (state *state) EntrypointHandler() http.Handler {
	return &state.entrypoint
}

// AutoCertProvider returns the autocert provider.
//
// If the autocert provider is not configured, it returns nil.
func (state *state) AutoCertProvider() server.CertProvider {
	return state.autocertProvider
}

func (state *state) LoadOrStoreProvider(key string, value types.RouteProvider) (actual types.RouteProvider, loaded bool) {
	actual, loaded = state.providers.LoadOrStore(key, value)
	return
}

func (state *state) DeleteProvider(key string) {
	state.providers.Delete(key)
}

func (state *state) IterProviders() iter.Seq2[string, types.RouteProvider] {
	return func(yield func(string, types.RouteProvider) bool) {
		for k, v := range state.providers.Range {
			if !yield(k, v) {
				return
			}
		}
	}
}

func (state *state) NumProviders() int {
	return state.providers.Size()
}

// this one is connection level access logger, different from entrypoint access logger
func (state *state) initAccessLogger() error {
	if !state.ACL.Valid() {
		return nil
	}
	return state.ACL.Start(state.task)
}

func (state *state) initEntrypoint() error {
	epCfg := state.Entrypoint
	matchDomains := state.MatchDomains

	state.entrypoint.SetFindRouteDomains(matchDomains)
	state.entrypoint.SetCatchAllRules(epCfg.Rules.CatchAll)
	state.entrypoint.SetNotFoundRules(epCfg.Rules.NotFound)

	errs := gperr.NewBuilder("entrypoint error")
	errs.Add(state.entrypoint.SetMiddlewares(epCfg.Middlewares))
	errs.Add(state.entrypoint.SetAccessLogger(state.task, epCfg.AccessLog))
	return errs.Error()
}

func (state *state) initMaxMind() error {
	maxmindCfg := state.Providers.MaxMind
	if maxmindCfg != nil {
		return maxmind.SetInstance(state.task, maxmindCfg)
	}
	return nil
}

func (state *state) initNotification() error {
	notifCfg := state.Providers.Notification
	if len(notifCfg) == 0 {
		return nil
	}

	dispatcher := notif.StartNotifDispatcher(state.task)
	for _, notifier := range notifCfg {
		dispatcher.RegisterProvider(&notifier)
	}
	return nil
}

func (state *state) initAutoCert() error {
	autocertCfg := state.AutoCert
	if autocertCfg == nil {
		autocertCfg = new(autocert.Config)
	}

	user, legoCfg, err := autocertCfg.GetLegoConfig()
	if err != nil {
		return err
	}

	state.autocertProvider = autocert.NewProvider(autocertCfg, user, legoCfg)
	if err := state.autocertProvider.Setup(); err != nil {
		return fmt.Errorf("autocert error: %w", err)
	} else {
		state.autocertProvider.ScheduleRenewal(state.task)
	}
	return nil
}

func (state *state) initProxmox() error {
	proxmoxCfg := state.Providers.Proxmox
	if len(proxmoxCfg) == 0 {
		return nil
	}

	errs := gperr.NewBuilder()
	for _, cfg := range proxmoxCfg {
		if err := cfg.Init(); err != nil {
			errs.Add(err.Subject(cfg.URL))
		}
	}
	return errs.Error()
}

func (state *state) storeProvider(p types.RouteProvider) {
	state.providers.Store(p.String(), p)
}

func (state *state) loadRouteProviders() error {
	providers := &state.Providers
	errs := gperr.NewBuilderWithConcurrency("route provider errors")
	results := gperr.NewBuilder("loaded route providers")

	agent.RemoveAllAgents()

	numProviders := len(providers.Agents) + len(providers.Files) + len(providers.Docker)
	providersCh := make(chan types.RouteProvider, numProviders)

	// start providers concurrently
	var providersConsumer sync.WaitGroup
	providersConsumer.Go(func() {
		for p := range providersCh {
			if actual, loaded := state.providers.LoadOrStore(p.String(), p); loaded {
				errs.Add(gperr.Errorf("provider %s already exists, first: %s, second: %s", p.String(), actual.GetType(), p.GetType()))
				continue
			}
			state.storeProvider(p)
		}
	})

	var providersProducer sync.WaitGroup
	for _, a := range providers.Agents {
		providersProducer.Go(func() {
			if err := a.Start(state.task.Context()); err != nil {
				errs.Add(gperr.PrependSubject(a.String(), err))
				return
			}
			agent.AddAgent(&a)
			p := route.NewAgentProvider(&a)
			providersCh <- p
		})
	}

	for _, filename := range providers.Files {
		providersProducer.Go(func() {
			p, err := route.NewFileProvider(filename)
			if err != nil {
				errs.Add(gperr.PrependSubject(filename, err))
			} else {
				providersCh <- p
			}
		})
	}

	for name, dockerHost := range providers.Docker {
		providersProducer.Go(func() {
			providersCh <- route.NewDockerProvider(name, dockerHost)
		})
	}

	providersProducer.Wait()

	close(providersCh)
	providersConsumer.Wait()

	lenLongestName := 0
	for k := range state.providers.Range {
		if len(k) > lenLongestName {
			lenLongestName = len(k)
		}
	}

	results.EnableConcurrency()

	// load routes concurrently
	var providersLoader sync.WaitGroup
	for _, p := range state.providers.Range {
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

func (state *state) startRouteProviders() error {
	errs := gperr.NewGroup("provider errors")
	for _, p := range state.providers.Range {
		errs.Go(func() error {
			return p.Start(state.Task())
		})
	}
	return errs.Wait().Error()
}
