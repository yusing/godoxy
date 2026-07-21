package route

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net"
	"runtime"
	"strings"
	"sync"

	"github.com/rs/zerolog"
	"github.com/yusing/godoxy/internal/agentpool"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/health"
	"github.com/yusing/godoxy/internal/homepage"
	idlewatcher "github.com/yusing/godoxy/internal/idlewatcher/runtime"
	"github.com/yusing/godoxy/internal/loadbalancer"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/proxmox"
	"github.com/yusing/godoxy/internal/routing"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/goutils/task"

	"github.com/yusing/godoxy/internal/logging/accesslog"
	"github.com/yusing/godoxy/internal/route/rules"
)

type (
	Route struct {
		// Alias is route lookup key.
		//
		// Supported HTTP host-match forms:
		//   - short alias: "app"
		//   - FQDN alias: "app.example.com"
		//   - leading-label wildcard alias: "*.example.com"
		//
		// Wildcard aliases match exactly one leftmost label and are checked only
		// after normal exact/domain lookup misses, preserving the fast path for
		// exact routes.
		Alias  string `json:"alias"`
		Scheme Scheme `json:"scheme,omitempty" swaggertype:"string" enums:"http,https,h2c,tcp,udp,fileserver"`
		Host   string `json:"host,omitempty"`
		Port   Port   `json:"port"`

		Bind string `json:"bind,omitempty" validate:"omitempty,ip_addr" extensions:"x-nullable"`

		Root  string `json:"root,omitempty"`
		SPA   bool   `json:"spa,omitempty"`   // Single-page app mode: serves index for non-existent paths
		Index string `json:"index,omitempty"` // Index file to serve for single-page app mode

		types.HTTPConfig
		InboundMTLSProfile       string                         `json:"inbound_mtls_profile,omitempty"` // HTTP-based routes only: must match a configured inbound_mtls_profiles entry and is ignored when entrypoint.inbound_mtls_profile is set
		Rules                    rules.Rules                    `json:"rules,omitempty" extensions:"x-nullable"`
		RuleFile                 string                         `json:"rule_file,omitempty" extensions:"x-nullable"`
		HealthCheck              health.HealthCheckConfig       `json:"healthcheck,omitzero" extensions:"x-nullable"` // null on load-balancer routes
		LoadBalance              *loadbalancer.Config           `json:"load_balance,omitempty" extensions:"x-nullable"`
		Middlewares              map[string]types.LabelMap      `json:"middlewares,omitempty" extensions:"x-nullable"`
		Homepage                 *homepage.ItemConfig           `json:"homepage"`
		AccessLog                *accesslog.RequestLoggerConfig `json:"access_log,omitempty" extensions:"x-nullable"`
		RelayProxyProtocolHeader bool                           `json:"relay_proxy_protocol_header,omitempty"` // TCP only: relay PROXY protocol header to the destination
		TLSTermination           bool                           `json:"tls_termination,omitempty"`             // TCP only: terminate inbound TLS on the shared HTTPS listener before proxying plaintext to the destination
		Agent                    string                         `json:"agent,omitempty"`

		Proxmox *proxmox.NodeConfig `json:"proxmox,omitempty" extensions:"x-nullable"`

		Idlewatcher *idlewatcher.IdlewatcherConfig `json:"idlewatcher,omitempty" extensions:"x-nullable"`

		Metadata `deserialize:"-"`
	} // @name Route

	Metadata struct {
		/* Docker only */
		Container                 *docker.Container `json:"container,omitempty" extensions:"x-nullable"`
		CanResolveDockerProxyPort bool              `json:"-" deserialize:"-"`
		CheckedDockerProxyPort    bool              `json:"-" deserialize:"-"`

		Provider string `json:"provider,omitempty" extensions:"x-nullable"` // for backward compatibility

		RootFS fs.FS `json:"-" deserialize:"-"`

		// ForceConflictWin lets built-in routes intentionally replace user or
		// provider-discovered routes that share the same key.
		ForceConflictWin bool `json:"-" deserialize:"-"`

		LisURL   *nettypes.URL `json:"lurl,omitempty" swaggertype:"string" extensions:"x-nullable"`
		ProxyURL *nettypes.URL `json:"purl,omitempty" swaggertype:"string"`

		Excluded       bool           `json:"excluded,omitempty" extensions:"x-nullable"`
		ExcludedReason ExcludedReason `json:"excluded_reason,omitempty" swaggertype:"string" extensions:"x-nullable"`

		HealthMon health.HealthMonitor `json:"health,omitempty" swaggerignore:"true"`
		// for swagger
		HealthJSON *health.HealthJSON `json:",omitempty" form:"health"`

		impl routing.Route
		task *task.Task

		// ensure err is read after validation or start
		valErr   lockedError
		startErr lockedError

		provider routing.Provider

		agent *agentpool.Agent

		started      chan struct{}
		onceStart    sync.Once
		onceValidate sync.Once
	}
	Routes map[string]*Route
)

type lockedError struct {
	sync.Mutex
	err error
}

func (le *lockedError) Get() error {
	le.Lock()
	defer le.Unlock()
	return le.err
}

func (le *lockedError) Set(err error) {
	le.Lock()
	defer le.Unlock()
	le.err = err
}

func (le *lockedError) SetDo(fn func(*error)) {
	le.Lock()
	defer le.Unlock()
	fn(&le.err)
}

const DefaultHost = "localhost"

func (r Routes) Contains(alias string) bool {
	_, ok := r[alias]
	return ok
}

func (r *Route) RouteMiddlewares() map[string]types.LabelMap {
	return maps.Clone(r.Middlewares)
}

func (r *Route) Init(parent task.Parent, name string, needFinish bool) {
	r.task = parent.Subtask(name, needFinish)
}

func (r *Route) SetTask(task *task.Task) {
	r.task = task
}

func (r *Route) Validate() error {
	// wait for alias to be set
	if r.Alias == "" {
		return nil
	}

	r.onceValidate.Do(func() {
		r.started = make(chan struct{})
		// close the channel when the route is destroyed (if not closed yet).
		runtime.AddCleanup(r, func(ch chan struct{}) {
			select {
			case <-ch:
			default:
				close(ch)
			}
		}, r.started)

		if build == nil {
			r.valErr.Set(ErrBuilderNotInitialized)
			return
		}
		impl, agent, err := build(r)
		if err != nil {
			r.valErr.Set(err)
			return
		}
		r.impl = impl
		r.agent = agent
	})
	return r.valErr.Get()
}

func (r *Route) Impl() routing.Route {
	return r.impl
}

func (r *Route) Task() *task.Task {
	return r.task
}

func (r *Route) Start(parent task.Parent) error {
	r.onceStart.Do(func() {
		r.startErr.Set(r.start(parent))
	})
	return r.startErr.Get()
}

func (r *Route) start(parent task.Parent) error {
	if r.impl == nil { // should not happen
		return errors.New("route not initialized")
	}
	defer close(r.started)

	// skip checking for excluded routes
	excluded := r.ShouldExclude()
	if !excluded && !r.ForceConflictWin {
		if err := checkExists(parent.Context(), r); err != nil {
			return err
		}
	}

	if !excluded {
		if err := r.impl.Start(parent); err != nil {
			return err
		}
	} else {
		ep := routing.EntrypointFromCtx(parent.Context())
		if ep == nil {
			return errors.New("entrypoint not initialized")
		}

		r.task = parent.Subtask("excluded."+r.Name(), false)
		r.task.SetValue(health.DisplayNameKey{}, r.DisplayName())
		ep.ExcludedRoutes().Add(r.impl)
		r.task.OnCancel("remove_route_from_excluded", func() {
			ep.ExcludedRoutes().Del(r.impl)
		})
		if r.UseHealthCheck() {
			if newHealthMonitor == nil {
				return errors.New("health monitor factory not initialized")
			}
			r.HealthMon = newHealthMonitor(r.impl)
			err := r.HealthMon.Start(r.task)
			if err != nil {
				return err
			}
		}
	}
	if cont := r.ContainerInfo(); cont != nil {
		docker.RegisterContainerConfig(cont.ContainerID, cont.DockerCfg)
		r.task.OnCancel("unregister_container_config", func() {
			docker.UnregisterContainerConfig(cont.ContainerID)
		})
	}
	return nil
}

func (r *Route) Finish(reason any) {
	r.FinishAndWait(reason)
}

func (r *Route) FinishAndWait(reason any) {
	if r.impl == nil {
		return
	}
	r.task.FinishAndWait(reason)
	r.impl = nil
}

func (r *Route) Started() <-chan struct{} {
	return r.started
}

func (r *Route) GetProvider() routing.Provider {
	return r.provider
}

func (r *Route) SetProvider(p routing.Provider) {
	r.provider = p
	r.Provider = p.ShortName()
}

func (r *Route) ProviderName() string {
	return r.Provider
}

func (r *Route) ListenURL() *nettypes.URL {
	return r.LisURL
}

func (r *Route) TargetURL() *nettypes.URL {
	return r.ProxyURL
}

func (r *Route) References() []string {
	aliasRef, _, ok := strings.Cut(r.Alias, ".")
	if !ok {
		aliasRef = r.Alias
	}

	if r.Container != nil {
		if r.Container.ContainerName != aliasRef {
			return []string{r.Container.ContainerName, aliasRef, r.Container.Image.Name, r.Container.Image.Author}
		}
		return []string{r.Container.Image.Name, aliasRef, r.Container.Image.Author}
	}

	if r.Proxmox != nil {
		if len(r.Proxmox.Services) > 0 && r.Proxmox.Services[0] != aliasRef {
			if r.Proxmox.VMName != aliasRef {
				return []string{r.Proxmox.VMName, aliasRef, r.Proxmox.Services[0]}
			}
			return []string{r.Proxmox.Services[0], aliasRef}
		}
		if r.Proxmox.VMName != aliasRef {
			return []string{r.Proxmox.VMName, aliasRef}
		}
	}
	return []string{aliasRef}
}

// Name implements pool.Object.
func (r *Route) Name() string {
	return r.Alias
}

// Key implements pool.Object.
func (r *Route) Key() string {
	if r.UseLoadBalance() || r.ShouldExclude() {
		// for excluded routes and load balanced routes, use provider:alias[-container_id[:8]] as key to make them unique.
		if r.Container != nil {
			return r.Provider + ":" + r.Alias + "-" + r.Container.ContainerID[:8]
		}
		return r.Provider + ":" + r.Alias
	}
	// we need to use alias as key for non-excluded routes because it's being used for subdomain / fqdn lookup for http routes.
	return r.Alias
}

func (r *Route) Type() RouteType {
	switch r.Scheme {
	case SchemeHTTP, SchemeHTTPS, SchemeH2C, SchemeFileServer:
		return RouteTypeHTTP
	case SchemeTCP, SchemeUDP:
		return RouteTypeStream
	}
	panic(fmt.Errorf("unexpected scheme %s for alias %s", r.Scheme, r.Alias))
}

func (r *Route) GetAgent() *agentpool.Agent {
	if r.Container != nil && r.Container.Agent != nil {
		return r.Container.Agent
	}
	return r.agent
}

func (r *Route) IsAgent() bool {
	return r.GetAgent() != nil
}

func (r *Route) HealthMonitor() health.HealthMonitor {
	return r.HealthMon
}

func (r *Route) SetHealthMonitor(m health.HealthMonitor) {
	if r.HealthMon != nil && r.HealthMon != m {
		r.HealthMon.Finish("health monitor replaced")
	}
	r.HealthMon = m
}

func (r *Route) IdlewatcherConfig() *idlewatcher.Config {
	return r.Idlewatcher
}

func (r *Route) HealthCheckConfig() health.HealthCheckConfig {
	return r.HealthCheck
}

func (r *Route) LoadBalanceConfig() *loadbalancer.Config {
	return r.LoadBalance
}

func (r *Route) HomepageItem() homepage.Item {
	containerID := ""
	if r.Container != nil {
		containerID = r.Container.ContainerID
	}
	var proxmoxContainer *homepage.ProxmoxContainer
	if r.Proxmox != nil && r.Proxmox.VMID != nil && *r.Proxmox.VMID > 0 {
		proxmoxContainer = &homepage.ProxmoxContainer{
			Node: r.Proxmox.Node,
			VMID: *r.Proxmox.VMID,
		}
	}
	return homepage.Item{
		Alias:       r.Alias,
		Provider:    r.Provider,
		ItemConfig:  *r.Homepage,
		ContainerID: containerID,
		Proxmox:     proxmoxContainer,
	}.GetOverride()
}

func (r *Route) DisplayName() string {
	if r.Homepage == nil { // should only happen in tests, Validate() should initialize it
		return r.Alias
	}
	return r.Homepage.Name
}

var zeroURL nettypes.URL

func (r *Route) MarshalZerologObject(e *zerolog.Event) {
	lisURL := r.LisURL
	proxyURL := r.ProxyURL
	if lisURL == nil {
		lisURL = &zeroURL
	}
	if proxyURL == nil {
		proxyURL = &zeroURL
	}
	e.Str("alias", r.Alias)
	switch r.Scheme {
	case SchemeHTTP, SchemeHTTPS, SchemeH2C:
		e.Str("type", "reverse_proxy").
			Str("scheme", r.Scheme.String()).
			Str("bind", lisURL.Host).
			Str("target", proxyURL.String())
	case SchemeFileServer:
		e.Str("type", "file_server").
			Str("root", r.Root)
	default:
		e.Str("type", "stream").
			Str("scheme", lisURL.Scheme+"->"+proxyURL.Scheme)
		if stream, ok := r.impl.(interface{ LocalAddr() net.Addr }); ok {
			// listening port could be zero (random),
			// use LocalAddr() to get the actual listening host+port.
			e.Str("bind", stream.LocalAddr().String())
		} else {
			// not yet started
			e.Str("bind", lisURL.Host)
		}
		e.Str("target", proxyURL.String())
	}
	if r.Proxmox != nil {
		e.Str("proxmox", r.Proxmox.Node)
		if r.Proxmox.VMID != nil {
			e.Uint64("vmid", *r.Proxmox.VMID)
		}
		if r.Proxmox.VMName != "" {
			e.Str("vmname", r.Proxmox.VMName)
		}
	}
	if r.Container != nil {
		e.Str("container", r.Container.ContainerName)
	}
}

// Base returns the base route object
func (r *Route) Base() *Route {
	return r
}

// PreferOver implements pool.Preferable to resolve duplicate route keys deterministically.
// Preference policy:
// - Prefer routes with rules over routes without rules.
// - If rules tie, prefer non-docker routes (explicit config) over docker-discovered routes.
// - Otherwise, prefer the new route to preserve existing semantics.
func (r *Route) PreferOver(other any) bool {
	// Try to get the underlying *Route of the other value
	var or *Route
	switch v := other.(type) {
	case *Route:
		or = v
	case interface{ Base() *Route }:
		or = v.Base()
	default:
		// Unknown type, allow replacement
		return true
	}

	// Prefer routes that have rules
	if len(r.Rules) > 0 && len(or.Rules) == 0 {
		return true
	}
	if len(r.Rules) == 0 && len(or.Rules) > 0 {
		return false
	}

	if r.ForceConflictWin && !or.ForceConflictWin {
		return true
	}
	if !r.ForceConflictWin && or.ForceConflictWin {
		return false
	}

	// Prefer explicit (non-docker) over docker auto-discovered
	if (r.Container == nil) != (or.Container == nil) {
		return r.Container == nil
	}

	// Default: allow replacement
	return true
}

func (r *Route) ContainerInfo() *docker.Container {
	return r.Container
}

func (r *Route) InboundMTLSProfileRef() string {
	return r.InboundMTLSProfile
}

func (r *Route) TerminatesTLS() bool {
	return r.TLSTermination
}

func (r *Route) IsDocker() bool {
	if r.Container == nil {
		return false
	}
	return r.Container.ContainerID != ""
}

func (r *Route) IsZeroPort() bool {
	return r.Port.Proxy == 0
}

func (r *Route) ShouldExclude() bool {
	if r.ExcludedReason != ExcludedReasonNone {
		return true
	}
	return r.FindExcludedReason() != ExcludedReasonNone
}

func (r *Route) FindExcludedReason() ExcludedReason {
	if r.valErr.Get() != nil {
		return ExcludedReasonError
	}
	if r.ExcludedReason != ExcludedReasonNone {
		return r.ExcludedReason
	}
	if r.Container != nil {
		switch {
		case r.Container.IsExcluded:
			return ExcludedReasonManual
		case r.CheckedDockerProxyPort && !r.CanResolveDockerProxyPort && !r.UseIdleWatcher():
			return ExcludedReasonNoPortContainer
		case !r.Container.IsExplicit && docker.IsBlacklisted(r.Container):
			return ExcludedReasonBlacklisted
		case strings.HasPrefix(r.Container.ContainerName, "buildx_"):
			return ExcludedReasonBuildx
		}
	}
	// this should happen on validation API only,
	// those routes are removed before validation.
	// see removeXPrefix in provider/file.go
	if strings.HasPrefix(r.Alias, "x-") { // for YAML anchors and references
		return ExcludedReasonYAMLAnchor
	}
	if strings.HasSuffix(r.Alias, "-old") {
		return ExcludedReasonOld
	}
	return ExcludedReasonNone
}

func (r *Route) UseLoadBalance() bool {
	return r.LoadBalance != nil && r.LoadBalance.Link != ""
}

func (r *Route) UseIdleWatcher() bool {
	return r.Idlewatcher != nil && r.Idlewatcher.IdleTimeout > 0 && r.Idlewatcher.ValErr() == nil
}

func (r *Route) UseHealthCheck() bool {
	if r.Container != nil {
		excludedReason := r.FindExcludedReason()
		switch {
		case r.Container.Image.Name == "godoxy-agent":
			return false
		case !r.Container.Running && !r.UseIdleWatcher():
			return false
		case strings.HasPrefix(r.Container.ContainerName, "buildx_"):
			return false
		case excludedReason == ExcludedReasonNoPortContainer:
			return false
		}
	}
	return !r.HealthCheck.Disable
}

func (r *Route) UseAccessLog() bool {
	return r.AccessLog != nil
}

// checkExists checks if the route already exists in the entrypoint.
//
// Context must be passed from the parent task that carries the entrypoint value.
func checkExists(ctx context.Context, r routing.Route) error {
	if r.UseLoadBalance() { // skip checking for load balanced routes
		return nil
	}
	ep := routing.EntrypointFromCtx(ctx)
	if ep == nil {
		return errors.New("entrypoint not found in context")
	}
	var (
		existing routing.Route
		ok       bool
	)
	switch r := r.(type) {
	case routing.HTTPRoute:
		existing, ok = routing.EntrypointFromCtx(ctx).HTTPRoutes().Get(r.Key())
	case routing.StreamRoute:
		existing, ok = routing.EntrypointFromCtx(ctx).StreamRoutes().Get(r.Key())
	}
	if ok {
		return fmt.Errorf("route already exists: from provider %s and %s", existing.ProviderName(), r.ProviderName())
	}
	return nil
}
