package config

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/goccy/go-yaml"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog"
	"github.com/yusing/godoxy/internal/acl"
	"github.com/yusing/godoxy/internal/agentpool"
	"github.com/yusing/godoxy/internal/autocert"
	config "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/entrypoint"
	homepage "github.com/yusing/godoxy/internal/homepage/types"
	"github.com/yusing/godoxy/internal/logging"
	"github.com/yusing/godoxy/internal/maxmind"
	"github.com/yusing/godoxy/internal/notif"
	route "github.com/yusing/godoxy/internal/route/provider"
	"github.com/yusing/godoxy/internal/route/routes"
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

	// used for temporary logging
	// discarded on failed reload
	tmpLogBuf *bytes.Buffer
	tmpLog    zerolog.Logger
}

func NewState() config.State {
	tmpLogBuf := bytes.NewBuffer(make([]byte, 0, 4096))
	return &state{
		providers:  xsync.NewMap[string, types.RouteProvider](),
		entrypoint: entrypoint.NewEntrypoint(),
		task:       task.RootTask("config", false),
		tmpLogBuf:  tmpLogBuf,
		tmpLog:     logging.NewLoggerWithFixedLevel(zerolog.InfoLevel, tmpLogBuf),
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
	config.ActiveState.Store(state)
	entrypoint.ActiveConfig.Store(&cfg.Entrypoint)
	homepage.ActiveConfig.Store(&cfg.Homepage)
	if autocertProvider := state.AutoCertProvider(); autocertProvider != nil {
		autocert.ActiveProvider.Store(autocertProvider.(*autocert.Provider))
	} else {
		autocert.ActiveProvider.Store(nil)
	}
}

func HasState() bool {
	return config.ActiveState.Load() != nil
}

func Value() *config.Config {
	return config.ActiveState.Load().Value()
}

func (state *state) InitFromFile(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			state.Config = config.DefaultConfig()
		} else {
			return err
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

func (state *state) ShortLinkMatcher() config.ShortLinkMatcher {
	return state.entrypoint.ShortLinkMatcher()
}

// AutoCertProvider returns the autocert provider.
//
// If the autocert provider is not configured, it returns nil.
func (state *state) AutoCertProvider() server.CertProvider {
	if state.autocertProvider == nil {
		return nil
	}
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

func (state *state) StartProviders() error {
	errs := gperr.NewGroup("provider errors")
	for _, p := range state.providers.Range {
		errs.Go(func() error {
			return p.Start(state.Task())
		})
	}
	return errs.Wait().Error()
}

func (state *state) NumProviders() int {
	return state.providers.Size()
}

func (state *state) FlushTmpLog() {
	state.tmpLogBuf.WriteTo(os.Stdout)
	state.tmpLogBuf.Reset()
}

// this one is connection level access logger, different from entrypoint access logger
func (state *state) initAccessLogger() error {
	if !state.ACL.Valid() {
		return nil
	}
	err := state.ACL.Start(state.task)
	if err != nil {
		return err
	}
	state.task.SetValue(acl.ContextKey{}, state.ACL)
	return nil
}

func (state *state) initEntrypoint() error {
	epCfg := state.Config.Entrypoint
	matchDomains := state.MatchDomains

	state.entrypoint.SetFindRouteDomains(matchDomains)
	state.entrypoint.SetNotFoundRules(epCfg.Rules.NotFound)

	if len(matchDomains) > 0 {
		state.entrypoint.ShortLinkMatcher().SetDefaultDomainSuffix(matchDomains[0])
	}

	if state.autocertProvider != nil {
		if domain := getAutoCertDefaultDomain(state.autocertProvider); domain != "" {
			state.entrypoint.ShortLinkMatcher().SetDefaultDomainSuffix("." + domain)
		}
	}

	errs := gperr.NewBuilder("entrypoint error")
	errs.Add(state.entrypoint.SetMiddlewares(epCfg.Middlewares))
	errs.Add(state.entrypoint.SetAccessLogger(state.task, epCfg.AccessLog))
	return errs.Error()
}

func getAutoCertDefaultDomain(p *autocert.Provider) string {
	if p == nil {
		return ""
	}
	cert, err := tls.LoadX509KeyPair(p.GetCertPath(), p.GetKeyPath())
	if err != nil || len(cert.Certificate) == 0 {
		return ""
	}
	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return ""
	}

	domain := x509Cert.Subject.CommonName
	if domain == "" && len(x509Cert.DNSNames) > 0 {
		domain = x509Cert.DNSNames[0]
	}
	domain = strings.TrimSpace(domain)
	if after, ok := strings.CutPrefix(domain, "*."); ok {
		domain = after
	}
	return strings.ToLower(domain)
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
		dispatcher.RegisterProvider(notifier)
	}
	return nil
}

func (state *state) initAutoCert() error {
	autocertCfg := state.AutoCert
	if autocertCfg == nil {
		autocertCfg = new(autocert.Config)
		_ = autocertCfg.Validate()
	}

	user, legoCfg, err := autocertCfg.GetLegoConfig()
	if err != nil {
		return err
	}

	p, err := autocert.NewProvider(autocertCfg, user, legoCfg)
	if err != nil {
		return err
	}

	if err := p.ObtainCertIfNotExistsAll(); err != nil {
		return err
	}

	p.ScheduleRenewalAll(state.task)
	p.PrintCertExpiriesAll()

	state.autocertProvider = p
	return nil
}

func (state *state) initProxmox() error {
	proxmoxCfg := state.Providers.Proxmox
	if len(proxmoxCfg) == 0 {
		return nil
	}

	var errs gperr.Group
	for _, cfg := range proxmoxCfg {
		errs.Go(func() error {
			if err := cfg.Init(state.task.Context()); err != nil {
				return err.Subject(cfg.URL)
			}
			return nil
		})
	}
	return errs.Wait().Error()
}

func (state *state) storeProvider(p types.RouteProvider) {
	state.providers.Store(p.String(), p)
}

func (state *state) loadRouteProviders() error {
	// disable pool logging temporary since we will have pretty logging below
	routes.HTTP.DisableLog(true)
	routes.Stream.DisableLog(true)

	defer func() {
		routes.HTTP.DisableLog(false)
		routes.Stream.DisableLog(false)
	}()

	providers := &state.Providers
	errs := gperr.NewGroup("route provider errors")
	results := gperr.NewGroup("loaded route providers")

	agentpool.RemoveAll()

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
			if err := a.Init(state.task.Context()); err != nil {
				errs.Add(gperr.PrependSubject(a.String(), err))
				return
			}
			agentpool.Add(a)
			p := route.NewAgentProvider(a)
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

	for name, dockerCfg := range providers.Docker {
		providersProducer.Go(func() {
			providersCh <- route.NewDockerProvider(name, dockerCfg)
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

	state.tmpLog.Info().Msg(results.Wait().String())
	state.printRoutesByProvider(lenLongestName)
	state.printState()
	return errs.Wait().Error()
}

func (state *state) printRoutesByProvider(lenLongestName int) {
	var routeResults strings.Builder
	routeResults.Grow(4096) // more than enough
	routeResults.WriteString("routes by provider\n")

	lenLongestName += 2 // > + space
	for _, p := range state.providers.Range {
		providerName := p.String()
		routeCount := p.NumRoutes()

		// Print provider header
		fmt.Fprintf(&routeResults, "> %-"+strconv.Itoa(lenLongestName)+"s %d routes:\n", providerName, routeCount)

		if routeCount == 0 {
			continue
		}

		// calculate longest name
		for alias, r := range p.IterRoutes {
			if r.ShouldExclude() {
				continue
			}
			displayName := r.DisplayName()
			if displayName != alias {
				displayName = fmt.Sprintf("%s (%s)", displayName, alias)
			}
			if len(displayName)+3 > lenLongestName { // 3 spaces + "-"
				lenLongestName = len(displayName) + 3
			}
		}

		for alias, r := range p.IterRoutes {
			if r.ShouldExclude() {
				continue
			}
			displayName := r.DisplayName()
			if displayName != alias {
				displayName = fmt.Sprintf("%s (%s)", displayName, alias)
			}
			fmt.Fprintf(&routeResults, "  - %-"+strconv.Itoa(lenLongestName-2)+"s -> %s\n", displayName, r.TargetURL().String())
		}
	}

	// Always print the routes since we want to show even empty providers
	routeStr := routeResults.String()
	if routeStr != "" {
		state.tmpLog.Info().Msg(routeStr)
	}
}

func (state *state) printState() {
	state.tmpLog.Info().Msg("active config:")
	yamlRepr, _ := yaml.Marshal(state.Config)
	state.tmpLog.Info().Msgf("%s", yamlRepr) // prevent copying when casting to string
}
