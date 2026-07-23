package config

import (
	"cmp"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"net"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/goccy/go-yaml"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	acl "github.com/yusing/godoxy/internal/acl/types"
	"github.com/yusing/godoxy/internal/agentpool"
	"github.com/yusing/godoxy/internal/api"
	"github.com/yusing/godoxy/internal/autocert"
	autocertctx "github.com/yusing/godoxy/internal/autocert/types"
	"github.com/yusing/godoxy/internal/common"
	config "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/entrypoint"
	entrypointctx "github.com/yusing/godoxy/internal/entrypoint"
	"github.com/yusing/godoxy/internal/health"
	"github.com/yusing/godoxy/internal/homepage"
	"github.com/yusing/godoxy/internal/logging"
	"github.com/yusing/godoxy/internal/maxmind"
	"github.com/yusing/godoxy/internal/notif"
	"github.com/yusing/godoxy/internal/proxmox"
	"github.com/yusing/godoxy/internal/route"
	routeimpl "github.com/yusing/godoxy/internal/route"
	provider "github.com/yusing/godoxy/internal/route/provider"
	"github.com/yusing/godoxy/internal/route/rules"
	rulepresets "github.com/yusing/godoxy/internal/route/rules/presets"
	"github.com/yusing/godoxy/internal/routing"

	"github.com/yusing/godoxy/internal/serialization"
	"github.com/yusing/godoxy/webui"
	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/server"
	"github.com/yusing/goutils/synk"
	"github.com/yusing/goutils/task"
)

type state struct {
	config.Config

	providers        *xsync.Map[string, routing.Provider]
	autocertProvider *autocert.Provider
	entrypoint       *entrypoint.Entrypoint
	agentPool        *agentpool.Pool

	task *task.Task

	status     synk.Value[config.RuntimeStatus]
	activation synk.Value[config.ActivationReport]

	preparationIssues   []config.ActivationIssue
	providerPreparation []routing.ProviderActivation

	// used for temporary logging
	// discarded on failed reload
	tmpLogBuf *logging.Buffer
	tmpLog    zerolog.Logger
}

// RejectingError prevents configuration acceptance.
//
// It is reserved for errors that make the candidate configuration unusable as
// a runtime definition. Provider and route activation failures are not
// RejectingError values.
type RejectingError struct {
	err error
}

func (e RejectingError) Error() string {
	return e.err.Error()
}

func (e RejectingError) Unwrap() error {
	return e.err
}

func NewState() *state {
	tmpLogBuf, tmpLog := logging.NewBufferedLogger(zerolog.InfoLevel)
	state := &state{
		providers: xsync.NewMap[string, routing.Provider](),
		task:      task.RootTask("config", false),
		agentPool: agentpool.NewPool(),
		tmpLogBuf: tmpLogBuf,
		tmpLog:    tmpLog,
	}
	state.status.Store(config.RuntimePreparing)
	config.SetCtx(state.task, state)
	agentpool.SetCtx(state.task, state.agentPool)
	proxmox.SetCtx(state.task, proxmox.NewNodePool())
	return state
}

func (state *state) InitFromFile(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			state.Config = config.DefaultConfig()
		} else {
			state.addPreparationIssue("config", config.IssueRejecting, err)
			return RejectingError{err}
		}
	}
	return state.Init(data)
}

func (state *state) Init(data []byte) error {
	state.preparationIssues = nil

	err := serialization.UnmarshalValidate(data, &state.Config, yaml.Unmarshal)
	if err != nil {
		state.addPreparationIssue("config", config.IssueRejecting, err)
		return RejectingError{err}
	}

	var errs []error
	if err := state.prepareComponent("notifications", config.IssueDegraded, state.initNotification); err != nil {
		errs = append(errs, err)
	}

	optionalComponents := [...]struct {
		name string
		init func() error
	}{
		{name: "maxmind", init: state.initMaxMind},
		{name: "proxmox", init: state.initProxmox},
		{name: "autocert", init: state.initAutoCert},
	}
	optionalResults := make([]error, len(optionalComponents))
	var wg sync.WaitGroup
	for i, component := range optionalComponents {
		wg.Go(func() {
			optionalResults[i] = component.init()
		})
	}
	wg.Wait()
	for i, err := range optionalResults {
		if err != nil {
			state.addPreparationIssue(optionalComponents[i].name, config.IssueDegraded, err)
			errs = append(errs, err)
		}
	}

	if err := state.prepareComponent("acl", config.IssueRejecting, state.initACL); err != nil {
		errs = append(errs, RejectingError{err})
	}
	if err := state.prepareComponent("entrypoint", config.IssueRejecting, state.initEntrypoint); err != nil {
		errs = append(errs, RejectingError{err})
	}
	// Provider construction and route loading failures are represented by the
	// structured provider activation report. Do not also add one aggregate
	// preparation issue for the same failures.
	if err := state.loadRouteProviders(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (state *state) prepareComponent(component string, severity config.IssueSeverity, fn func() error) error {
	err := fn()
	if err != nil {
		state.addPreparationIssue(component, severity, err)
	}
	return err
}

func (state *state) addPreparationIssue(component string, severity config.IssueSeverity, err error) {
	state.preparationIssues = append(state.preparationIssues, config.ActivationIssue{
		Component: component,
		Severity:  severity,
		Err:       gperr.Wrap(err),
	})
}

func (state *state) PreparationIssues() []config.ActivationIssue {
	return slices.Clone(state.preparationIssues)
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

func (state *state) Entrypoint() routing.Entrypoint {
	return state.entrypoint
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

func (state *state) LoadOrStoreProvider(key string, value routing.Provider) (actual routing.Provider, loaded bool) {
	actual, loaded = state.providers.LoadOrStore(key, value)
	return
}

func (state *state) DeleteProvider(key string) {
	state.providers.Delete(key)
}

func (state *state) IterProviders() iter.Seq2[string, routing.Provider] {
	return func(yield func(string, routing.Provider) bool) {
		for k, v := range state.providers.Range {
			if !yield(k, v) {
				return
			}
		}
	}
}

func (state *state) ActivateProviders(parent task.Parent) routing.ProviderActivationReport {
	if parent == nil {
		parent = state.Task()
	}
	if state.entrypoint != nil {
		// Route inventory diagnostics already summarize the initial bulk load.
		// Pool event history remains enabled while duplicate console records are
		// suppressed, and later dynamic additions are logged normally.
		state.entrypoint.DisablePoolsLog(true)
		defer state.entrypoint.DisablePoolsLog(false)
	}
	providers := make([]routing.Provider, 0, state.providers.Size())
	for _, p := range state.providers.Range {
		providers = append(providers, p)
	}
	activations := make([]routing.ProviderActivation, len(providers))
	var wg sync.WaitGroup
	for i, p := range providers {
		wg.Go(func() {
			activations[i] = p.Activate(parent)
		})
	}
	wg.Wait()

	var report routing.ProviderActivationReport
	for _, activation := range state.providerPreparation {
		report.Add(activation)
	}
	for _, activation := range activations {
		report.Add(activation)
	}
	return report
}

func (state *state) NumProviders() int {
	return state.providers.Size()
}

func (state *state) FlushTmpLog() error {
	firstErr := state.tmpLogBuf.Flush()
	if firstErr == nil {
		return nil
	}
	if retryErr := state.tmpLogBuf.Flush(); retryErr != nil {
		state.tmpLogBuf.Passthrough()
		return fmt.Errorf("flush configuration diagnostics: %w", errors.Join(firstErr, retryErr))
	}
	return nil
}

func (state *state) discardTmpLog() {
	state.tmpLogBuf.Discard()
}

func (state *state) LoadLogger() *zerolog.Logger {
	return &state.tmpLog
}

func (state *state) ActivateAPIServers(parent task.Parent) config.APIActivationReport {
	if parent == nil {
		parent = state.Task()
	}
	report := config.APIActivationReport{
		Main: config.ComponentActivation{Configured: true, Required: true},
		Local: config.ComponentActivation{
			Configured: common.LocalAPIHTTPAddr != "",
		},
	}
	if cause := context.Cause(parent.Context()); cause != nil {
		report.Main.Err = gperr.Wrap(cause)
		if report.Local.Configured {
			report.Local.Err = gperr.Wrap(cause)
		}
		return report
	}

	// API Handler needs to start after auth is initialized.
	report.Main.Ready, report.Main.Err = activateAPIServer(parent, "api_server", server.Options{
		Name:     "api",
		HTTPAddr: common.APIHTTPAddr,
		Handler:  api.NewHandler(true),
	})

	// Local API Handler is used for unauthenticated access.
	if common.LocalAPIHTTPAddr != "" {
		if err := validateLocalAPIAddr(common.LocalAPIHTTPAddr, common.LocalAPIAllowNonLoopback); err != nil {
			report.Local.Err = gperr.Wrap(err)
			return report
		}
		if common.LocalAPIAllowNonLoopback && !isLoopbackLocalAPIHost(common.LocalAPIHTTPAddr) {
			log.Warn().
				Str("addr", common.LocalAPIHTTPAddr).
				Msg("local API server is allowed to bind to non-loopback addresses")
		}
		report.Local.Ready, report.Local.Err = activateAPIServer(parent, "local_api_server", server.Options{
			Name:     "local_api",
			HTTPAddr: common.LocalAPIHTTPAddr,
			Handler:  api.NewHandler(false),
		})
	}
	return report
}

func activateAPIServer(parent task.Parent, taskName string, opts server.Options) (bool, gperr.Error) {
	if cause := context.Cause(parent.Context()); cause != nil {
		return false, gperr.Wrap(cause)
	}

	serverTask := parent.Subtask(taskName, false)
	_, err := server.StartServer(serverTask, opts)
	if err != nil {
		serverTask.FinishAndWait(err)
		return false, gperr.Wrap(err)
	}
	return true, nil
}

func validateLocalAPIAddr(addr string, allowNonLoopback bool) error {
	if isLoopbackLocalAPIHost(addr) {
		return nil
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}

	if allowNonLoopback {
		return nil
	}

	switch strings.ToLower(host) {
	case "localhost":
		return nil
	case "":
		return errors.New("local API address must bind to a loopback host, not all interfaces")
	}

	ip, err := netip.ParseAddr(host)
	if err != nil {
		return fmt.Errorf("local API address must use a loopback host: %w", err)
	}
	if !ip.IsLoopback() {
		return fmt.Errorf("local API address must bind to a loopback host, got %q", host)
	}
	return nil
}

func isLoopbackLocalAPIHost(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}

	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

func (state *state) setActivation(report config.ActivationReport, health config.ActivationHealth) {
	state.activation.Store(report)
	switch health {
	case config.ActivationHealthy:
		state.status.Store(config.RuntimeHealthy)
	case config.ActivationDegraded:
		state.status.Store(config.RuntimeDegraded)
	case config.ActivationFailed:
		state.status.Store(config.RuntimeFailed)
	}
}

func (state *state) setStatus(status config.RuntimeStatus) {
	state.status.Store(status)
}

func (state *state) RuntimeSnapshot() config.RuntimeSnapshot {
	status := state.status.Load()
	var health config.ActivationHealth
	switch status {
	case config.RuntimeHealthy:
		health = config.ActivationHealthy
	case config.RuntimeDegraded:
		health = config.ActivationDegraded
	case config.RuntimeFailed:
		health = config.ActivationFailed
	}
	return config.RuntimeSnapshot{
		Status:     status,
		Health:     health,
		Activation: state.activation.Load(),
	}
}

func (state *state) Stop(reason any) {
	state.status.Store(config.RuntimeStopping)
	state.task.FinishAndWait(reason)
}

// initACL initializes the ACL.
func (state *state) initACL() error {
	if !state.ACL.Valid() {
		return nil
	}
	err := state.ACL.Start(state.task)
	if err != nil {
		return err
	}
	acl.SetCtx(state.task, state.ACL)
	return nil
}

func (state *state) initEntrypoint() error {
	epCfg := state.Config.Entrypoint
	matchDomains := state.MatchDomains
	if warning := proxyProtocolDeprecationWarning(epCfg); warning != "" {
		state.tmpLog.Warn().Msg(warning)
	}

	state.entrypoint = entrypoint.NewEntrypoint(state.task, &epCfg)
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

	entrypointctx.SetCtx(state.task, state.entrypoint)

	errs := gperr.NewBuilder("entrypoint error")
	errs.Add(state.entrypoint.SetMiddlewares(epCfg.Middlewares))
	errs.Add(state.entrypoint.SetAccessLogger(state.task, epCfg.AccessLog))
	errs.Add(state.entrypoint.SetInboundMTLSProfiles(state.Config.InboundMTLSProfiles))
	return errs.Error()
}

func proxyProtocolDeprecationWarning(cfg entrypoint.Config) string {
	if !cfg.SupportProxyProtocol {
		return ""
	}
	if cfg.ProxyProtocol == nil {
		return "entrypoint.support_proxy_protocol is deprecated and trusts PROXY headers from any peer; configure entrypoint.proxy_protocol with mode required or mixed and at least one trusted_proxies entry"
	}
	return "entrypoint.support_proxy_protocol is deprecated and ignored because entrypoint.proxy_protocol is configured; remove the deprecated setting"
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
	if maxmindCfg == nil {
		return nil
	}

	instance, err := maxmind.New(state.task, maxmindCfg)
	if err != nil {
		return err
	}
	maxmind.SetCtx(state.task, instance)
	return nil
}

func (state *state) initNotification() error {
	notifCfg := state.Providers.Notification
	if len(notifCfg) == 0 {
		return nil
	}

	dispatcher := notif.NewDispatcher(state.task)
	for _, notifier := range notifCfg {
		dispatcher.RegisterProvider(notifier)
	}
	notif.SetCtx(state.task, dispatcher)
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
	autocertctx.SetCtx(state.task, p)
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
				return gperr.PrependSubject(err, cfg.URL)
			}
			return nil
		})
	}
	return errs.Wait().Error()
}

func (state *state) loadRouteProviders() error {
	providers := state.Providers
	errs := gperr.NewBuilder("route provider errors")
	state.providerPreparation = nil

	state.agentPool.RemoveAll()

	registerProvider := func(p routing.Provider) {
		if actual, loaded := state.providers.LoadOrStore(p.String(), p); loaded {
			err := fmt.Errorf("provider %s already exists, first: %s, second: %s", p.String(), actual.GetType(), p.GetType())
			errs.Add(err)
			state.providerPreparation = append(state.providerPreparation, routing.ProviderActivation{
				Provider:            p.String(),
				InfrastructureError: gperr.Wrap(err),
			})
		}
	}

	agentErrs := gperr.NewGroup("agent init errors")
	for _, a := range providers.Agents {
		agentErrs.Go(func() error {
			if err := a.Init(state.task.Context()); err != nil {
				return gperr.PrependSubject(err, a.String())
			}
			state.agentPool.Add(a)
			return nil
		})
	}

	if err := agentErrs.Wait().Error(); err != nil {
		errs.Add(err)
	}

	for _, a := range providers.Agents {
		registerProvider(provider.NewAgentProvider(a))
	}

	for _, filename := range providers.Files {
		p, err := provider.NewFileProvider(filename)
		if err != nil {
			err = gperr.PrependSubject(err, filename)
			errs.Add(err)
			state.providerPreparation = append(state.providerPreparation, routing.ProviderActivation{
				Provider:            filename,
				InfrastructureError: gperr.Wrap(err),
			})
			continue
		}
		registerProvider(p)
	}

	for name, dockerCfg := range providers.Docker {
		registerProvider(provider.NewDockerProvider(name, dockerCfg))
	}

	lenLongestName := 0
	for k := range state.providers.Range {
		if len(k) > lenLongestName {
			lenLongestName = len(k)
		}
	}

	// load routes concurrently
	loadErrs := gperr.NewGroup("route load errors")

	results := gperr.NewBuilder("loaded route providers")
	providerList := make([]routing.Provider, 0, state.providers.Size())
	for _, p := range state.providers.Range {
		providerList = append(providerList, p)
	}
	loadedResults := make([]string, len(providerList))
	for i, p := range providerList {
		loadErrs.Go(func() error {
			if err := p.LoadRoutes(state.Context()); err != nil {
				return gperr.PrependSubject(err, p.String())
			}
			loadedResults[i] = fmt.Sprintf("%-"+strconv.Itoa(lenLongestName)+"s %d routes", p.String(), p.NumRoutes())
			return nil
		})
	}
	if err := loadErrs.Wait().Error(); err != nil {
		errs.Add(err)
	}
	for _, result := range loadedResults {
		if result != "" {
			results.Adds(result)
		}
	}

	if err := state.initWebUIRoute(); err != nil {
		errs.Add(err)
		state.providerPreparation = append(state.providerPreparation, routing.ProviderActivation{
			Provider:            "webui",
			InfrastructureError: gperr.Wrap(err),
		})
	}

	state.LogProxmoxDiscoveries(state.proxmoxDiscoveries())
	state.logLoadedRouteProviders(results.String())
	state.printRoutesByProvider(lenLongestName)
	state.logStartupSummary()
	return errs.Error()
}

func (state *state) LogProxmoxDiscoveries(discoveries []proxmox.Discovery) {
	if len(discoveries) == 0 {
		return
	}
	discoveries = slices.Clone(discoveries)

	slices.SortFunc(discoveries, func(a, b proxmox.Discovery) int {
		if a.Node != b.Node {
			return strings.Compare(a.Node, b.Node)
		}
		if a.Alias != b.Alias {
			return strings.Compare(a.Alias, b.Alias)
		}
		if a.Kind != b.Kind {
			return strings.Compare(string(a.Kind), string(b.Kind))
		}
		return cmp.Compare(a.VMID, b.VMID)
	})

	longestName := 0
	for _, discovery := range discoveries {
		longestName = max(longestName, len(proxmoxDiscoveryName(discovery)))
	}

	var result strings.Builder
	result.Grow(len(discoveries) * 64)
	result.WriteString("discovered proxmox routes\n")
	for start := 0; start < len(discoveries); {
		node := discoveries[start].Node
		end := start + 1
		for end < len(discoveries) && discoveries[end].Node == node {
			end++
		}

		count := end - start
		noun := "routes"
		if count == 1 {
			noun = "route"
		}
		fmt.Fprintf(&result, "> %s %d %s:\n", diagnosticText(node, "unknown node"), count, noun)
		for _, discovery := range discoveries[start:end] {
			fmt.Fprintf(
				&result,
				"  - %-"+strconv.Itoa(longestName)+"s  %s\n",
				proxmoxDiscoveryName(discovery),
				proxmoxDiscoveryTarget(discovery),
			)
		}
		start = end
	}

	state.tmpLog.Info().Msg(result.String())
}

type proxmoxDiscoveryRoute interface {
	ProxmoxDiscovery() (proxmox.Discovery, bool)
}

func (state *state) proxmoxDiscoveries() []proxmox.Discovery {
	var discoveries []proxmox.Discovery
	for _, provider := range state.providers.Range {
		for _, rt := range provider.IterRoutes {
			source, ok := rt.(proxmoxDiscoveryRoute)
			if !ok {
				continue
			}
			if discovery, ok := source.ProxmoxDiscovery(); ok {
				discoveries = append(discoveries, discovery)
			}
		}
	}
	return discoveries
}

func proxmoxDiscoveryName(discovery proxmox.Discovery) string {
	alias := diagnosticText(discovery.Alias, "unnamed route")
	if discovery.VMName == "" || discovery.VMName == discovery.Alias {
		return alias
	}
	return fmt.Sprintf("%s (%s)", diagnosticText(discovery.VMName, "unnamed resource"), alias)
}

func proxmoxDiscoveryTarget(discovery proxmox.Discovery) string {
	kind := diagnosticText(string(discovery.Kind), "unknown")
	var target string
	if discovery.Kind == proxmox.DiscoveryNode {
		target = kind
	} else {
		target = fmt.Sprintf("%s %d", kind, discovery.VMID)
	}
	if discovery.Target != "" {
		target += " -> " + diagnosticText(discovery.Target, "unknown target")
	}
	return target
}

func diagnosticText(value, placeholder string) string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return "<" + placeholder + ">"
	}
	return value
}

func (state *state) initWebUIRoute() error {
	aliases := state.WebUI.Aliases
	if len(aliases) == 0 {
		aliases = common.FrontendAliasesLegacy
	}
	if len(aliases) == 0 {
		return nil
	}

	var webuiRoute *routeimpl.Route

	routes := make(routeimpl.Routes, len(aliases))
	for _, alias := range aliases {
		alias = strings.ToLower(strings.TrimSpace(alias))
		if alias == "" {
			continue
		}
		for providerName, p := range state.providers.Range {
			if providerName == "webui" {
				continue
			}
			if existing, ok := p.GetRoute(alias); ok {
				state.tmpLog.Warn().
					Str("alias", alias).
					Str("existing_provider", existing.ProviderName()).
					Msg("webui route conflicts with existing route; built-in webui route will be used")
			}
		}
		if webuiRoute == nil {
			r, err := state.newWebUIRoute()
			if err != nil {
				return err
			}
			webuiRoute = r
		}
		routes[alias] = webuiRoute
	}

	if len(routes) == 0 {
		return nil
	}

	webuiProvider := provider.NewStaticProvider("webui", routes)
	if err := webuiProvider.LoadRoutes(state.Context()); err != nil {
		return err
	}
	if actual, loaded := state.providers.LoadAndStore(webuiProvider.String(), webuiProvider); loaded {
		state.tmpLog.Warn().
			Str("provider", webuiProvider.String()).
			Str("existing_type", string(actual.GetType())).
			Msg("webui provider key already exists; replacing it with built-in webui route")
	}
	return nil
}

func (state *state) newWebUIRoute() (*routeimpl.Route, error) {
	webuiRules, err := loadWebUIRules("webui.yml", state.WebUI.Rules)
	if err != nil {
		return nil, err
	}

	r := routeimpl.Route{
		Scheme:      route.SchemeFileServer,
		Root:        "embed://webui",
		SPA:         true,
		Index:       "_shell.html",
		Rules:       webuiRules,
		HealthCheck: health.HealthCheckConfig{Disable: true},
		Homepage: &homepage.ItemConfig{
			Show: false,
		},
		Metadata: routeimpl.Metadata{
			Provider:         "webui",
			RootFS:           webui.Dist(),
			ForceConflictWin: true,
		},
		InboundMTLSProfile: state.WebUI.InboundMTLSProfile,
		Middlewares:        state.WebUI.Middlewares,
		AccessLog:          state.WebUI.AccessLog,
	}

	host, port, ok, err := webUIDevServerURL()
	if err != nil || !ok {
		return &r, err
	}

	r.Scheme = route.SchemeHTTP
	r.Host = host
	r.Port.Proxy = port
	r.Root = ""
	r.Rules, err = loadWebUIRules("webui_dev.yml", state.WebUI.Rules)
	if err != nil {
		return nil, err
	}
	r.SPA = false
	r.Index = ""
	r.Metadata.RootFS = nil
	state.tmpLog.Info().Msg("using WebUI Vite dev server")
	return &r, nil
}

func loadWebUIRules(presetName string, extra rules.Rules) (rules.Rules, error) {
	webuiRules, ok := rulepresets.GetRulePreset(presetName)
	if !ok {
		return nil, fmt.Errorf("rule preset %q not found", presetName)
	}
	webuiRules = slices.Clone(webuiRules)
	return append(webuiRules, extra...), nil
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

func (state *state) logLoadedRouteProviders(message string) {
	numRoutes := 0
	for _, p := range state.providers.Range {
		numRoutes += p.NumRoutes()
	}

	state.tmpLog.Info().
		Int("route_providers", state.providers.Size()).
		Int("routes", numRoutes).
		Msg(message)
}

func (state *state) logStartupSummary() {
	enabledSubsystems := make([]string, 0, 8)
	if state.ACL.Valid() {
		enabledSubsystems = append(enabledSubsystems, "acl")
	}
	if state.autocertProvider != nil {
		enabledSubsystems = append(enabledSubsystems, "autocert")
	}
	if len(state.Config.Entrypoint.Middlewares) > 0 {
		enabledSubsystems = append(enabledSubsystems, "entrypoint_middlewares")
	}
	if state.Config.Entrypoint.AccessLog != nil {
		enabledSubsystems = append(enabledSubsystems, "access_log")
	}
	if len(state.Config.InboundMTLSProfiles) > 0 {
		enabledSubsystems = append(enabledSubsystems, "inbound_mtls")
	}
	if len(state.Config.Providers.Notification) > 0 {
		enabledSubsystems = append(enabledSubsystems, "notifications")
	}
	if state.Config.Providers.MaxMind != nil {
		enabledSubsystems = append(enabledSubsystems, "maxmind")
	}
	if len(state.Config.Providers.Proxmox) > 0 {
		enabledSubsystems = append(enabledSubsystems, "proxmox")
	}

	state.tmpLog.Info().
		Strs("enabled_subsystems", enabledSubsystems).
		Msg("startup configuration summary")
}
