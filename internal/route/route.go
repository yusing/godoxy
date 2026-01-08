package route

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/agentpool"
	config "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/health/monitor"
	"github.com/yusing/godoxy/internal/homepage"
	homepagecfg "github.com/yusing/godoxy/internal/homepage/types"
	netutils "github.com/yusing/godoxy/internal/net"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/proxmox"
	"github.com/yusing/godoxy/internal/serialization"
	"github.com/yusing/godoxy/internal/types"
	gperr "github.com/yusing/goutils/errs"
	strutils "github.com/yusing/goutils/strings"
	"github.com/yusing/goutils/task"

	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/logging/accesslog"
	"github.com/yusing/godoxy/internal/route/routes"
	"github.com/yusing/godoxy/internal/route/rules"
	rulepresets "github.com/yusing/godoxy/internal/route/rules/presets"
	route "github.com/yusing/godoxy/internal/route/types"
)

type (
	Route struct {
		Alias  string       `json:"alias"`
		Scheme route.Scheme `json:"scheme,omitempty" swaggertype:"string" enums:"http,https,h2c,tcp,udp,fileserver"`
		Host   string       `json:"host,omitempty"`
		Port   route.Port   `json:"port"`

		// for TCP and UDP routes, bind address to listen on
		Bind string `json:"bind,omitempty" validate:"omitempty,ip_addr" extensions:"x-nullable"`

		Root  string `json:"root,omitempty"`
		SPA   bool   `json:"spa,omitempty"`   // Single-page app mode: serves index for non-existent paths
		Index string `json:"index,omitempty"` // Index file to serve for single-page app mode

		route.HTTPConfig
		PathPatterns []string                       `json:"path_patterns,omitempty" extensions:"x-nullable"`
		Rules        rules.Rules                    `json:"rules,omitempty" extension:"x-nullable"`
		RuleFile     string                         `json:"rule_file,omitempty" extensions:"x-nullable"`
		HealthCheck  types.HealthCheckConfig        `json:"healthcheck,omitempty" extensions:"x-nullable"` // null on load-balancer routes
		LoadBalance  *types.LoadBalancerConfig      `json:"load_balance,omitempty" extensions:"x-nullable"`
		Middlewares  map[string]types.LabelMap      `json:"middlewares,omitempty" extensions:"x-nullable"`
		Homepage     *homepage.ItemConfig           `json:"homepage"`
		AccessLog    *accesslog.RequestLoggerConfig `json:"access_log,omitempty" extensions:"x-nullable"`
		Agent        string                         `json:"agent,omitempty"`

		Idlewatcher *types.IdlewatcherConfig `json:"idlewatcher,omitempty" extensions:"x-nullable"`

		Metadata `deserialize:"-"`
	}

	Metadata struct {
		/* Docker only */
		Container *types.Container `json:"container,omitempty" extensions:"x-nullable"`

		Provider string `json:"provider,omitempty" extensions:"x-nullable"` // for backward compatibility

		// private fields
		LisURL   *nettypes.URL `json:"lurl,omitempty" swaggertype:"string" extensions:"x-nullable"`
		ProxyURL *nettypes.URL `json:"purl,omitempty" swaggertype:"string"`

		Excluded       bool           `json:"excluded,omitempty" extensions:"x-nullable"`
		ExcludedReason ExcludedReason `json:"excluded_reason,omitempty" swaggertype:"string" extensions:"x-nullable"`

		HealthMon types.HealthMonitor `json:"health,omitempty" swaggerignore:"true"`
		// for swagger
		HealthJSON *types.HealthJSON `json:",omitempty" form:"health"`

		impl types.Route
		task *task.Task

		// ensure err is read after validation or start
		valErr   lockedError
		startErr lockedError

		provider types.RouteProvider

		agent *agentpool.Agent

		started      chan struct{}
		onceStart    sync.Once
		onceValidate sync.Once
	}
	Routes map[string]*Route
	Port   = route.Port
)

type lockedError struct {
	err  gperr.Error
	lock sync.Mutex
}

func (le *lockedError) Get() gperr.Error {
	le.lock.Lock()
	defer le.lock.Unlock()
	return le.err
}

func (le *lockedError) Set(err gperr.Error) {
	le.lock.Lock()
	defer le.lock.Unlock()
	le.err = err
}

const DefaultHost = "localhost"

func (r Routes) Contains(alias string) bool {
	_, ok := r[alias]
	return ok
}

func (r *Route) Validate() gperr.Error {
	// pcs := make([]uintptr, 1)
	// runtime.Callers(2, pcs)
	// f := runtime.FuncForPC(pcs[0])
	// fname := f.Name()
	r.onceValidate.Do(func() {
		// filename, line := f.FileLine(pcs[0])
		// if strings.HasPrefix(r.Alias, "godoxy") {
		// 	log.Debug().Str("route", r.Alias).Str("caller", fname).Str("file", filename).Int("line", line).Msg("validating route")
		// }
		r.valErr.Set(r.validate())
	})
	return r.valErr.Get()
}

func (r *Route) validate() gperr.Error {
	// if strings.HasPrefix(r.Alias, "godoxy") {
	// 	log.Debug().Any("route", r).Msg("validating route")
	// }
	if r.Agent != "" {
		if r.Container != nil {
			return gperr.Errorf("specifying agent is not allowed for docker container routes")
		}
		var ok bool
		// by agent address
		r.agent, ok = agentpool.Get(r.Agent)
		if !ok {
			// fallback to get agent by name
			r.agent, ok = agentpool.GetAgent(r.Agent)
			if !ok {
				return gperr.Errorf("agent %s not found", r.Agent)
			}
		}
	}

	r.Finalize()

	r.started = make(chan struct{})
	// close the channel when the route is destroyed (if not closed yet).
	runtime.AddCleanup(r, func(ch chan struct{}) {
		select {
		case <-ch:
		default:
			close(ch)
		}
	}, r.started)

	if r.Idlewatcher != nil && r.Idlewatcher.Proxmox != nil {
		node := r.Idlewatcher.Proxmox.Node
		vmid := r.Idlewatcher.Proxmox.VMID
		if node == "" {
			return gperr.Errorf("node (proxmox node name) is required")
		}
		if vmid <= 0 {
			return gperr.Errorf("vmid (lxc id) is required")
		}
		if r.Host == DefaultHost {
			containerName := r.Idlewatcher.ContainerName()
			// get ip addresses of the vmid
			node, ok := proxmox.Nodes.Get(node)
			if !ok {
				return gperr.Errorf("proxmox node %s not found in pool", node)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			ips, err := node.LXCGetIPs(ctx, vmid)
			if err != nil {
				return gperr.Errorf("failed to get ip addresses of vmid %d: %w", vmid, err)
			}

			if len(ips) == 0 {
				return gperr.Multiline().
					Addf("no ip addresses found for %s", containerName).
					Adds("make sure you have set static ip address for container instead of dhcp").
					Subject(containerName)
			}

			l := log.With().Str("container", containerName).Logger()

			l.Info().Msg("checking if container is running")
			running, err := node.LXCIsRunning(ctx, vmid)
			if err != nil {
				return gperr.New("failed to check container state").With(err)
			}

			if !running {
				l.Info().Msg("starting container")
				if err := node.LXCAction(ctx, vmid, proxmox.LXCStart); err != nil {
					return gperr.New("failed to start container").With(err)
				}
			}

			l.Info().Msgf("finding reachable ip addresses")
			errs := gperr.NewBuilder("failed to find reachable ip addresses")
			for _, ip := range ips {
				if err := netutils.PingTCP(ctx, ip, r.Port.Proxy); err != nil {
					errs.Add(gperr.Unwrap(err).Subjectf("%s:%d", ip, r.Port.Proxy))
				} else {
					r.Host = ip.String()
					l.Info().Msgf("using ip %s", r.Host)
					break
				}
			}
			if r.Host == DefaultHost {
				return gperr.Multiline().
					Addf("no reachable ip addresses found, tried %d IPs", len(ips)).
					With(errs.Error()).
					Subject(containerName)
			}
		}
	}

	if r.Container != nil && r.Container.IdlewatcherConfig != nil {
		r.Idlewatcher = r.Container.IdlewatcherConfig
	}

	// return error if route is localhost:<godoxy_port> but route is not agent
	if !r.IsAgent() {
		switch r.Host {
		case "localhost", "127.0.0.1":
			switch r.Port.Proxy {
			case common.ProxyHTTPPort, common.ProxyHTTPSPort, common.APIHTTPPort:
				if r.Scheme.IsReverseProxy() || r.Scheme == route.SchemeTCP {
					return gperr.Errorf("localhost:%d is reserved for godoxy", r.Port.Proxy)
				}
			}
		}
	}

	var errs gperr.Builder
	if err := r.validateRules(); err != nil {
		errs.Add(err)
	}

	var impl types.Route
	var err gperr.Error

	switch r.Scheme {
	case route.SchemeFileServer:
		r.Host = ""
		r.Port.Proxy = 0
		r.ProxyURL = gperr.Collect(&errs, nettypes.ParseURL, "file://"+r.Root)
	case route.SchemeHTTP, route.SchemeHTTPS, route.SchemeH2C:
		if r.Port.Listening != 0 {
			errs.Addf("unexpected listening port for %s scheme", r.Scheme)
		}
		r.ProxyURL = gperr.Collect(&errs, nettypes.ParseURL, fmt.Sprintf("%s://%s:%d", r.Scheme, r.Host, r.Port.Proxy))
	case route.SchemeTCP, route.SchemeUDP:
		if !r.ShouldExclude() {
			if r.Bind == "" {
				r.Bind = "0.0.0.0"
			}
			bindIP := net.ParseIP(r.Bind)
			if bindIP == nil {
				return gperr.Errorf("invalid bind address %s", r.Bind)
			}
			remoteIP := net.ParseIP(r.Host)
			if remoteIP == nil {
				return gperr.Errorf("invalid remote address %s", r.Host)
			}
			toNetwork := func(ip net.IP, scheme route.Scheme) string {
				if ip.To4() == nil {
					if scheme == route.SchemeTCP {
						return "tcp6"
					}
					return "udp6"
				}
				if scheme == route.SchemeTCP {
					return "tcp4"
				}
				return "udp4"
			}
			lScheme := toNetwork(bindIP, r.Scheme)
			rScheme := toNetwork(remoteIP, r.Scheme)

			r.LisURL = gperr.Collect(&errs, nettypes.ParseURL, fmt.Sprintf("%s://%s:%d", lScheme, r.Bind, r.Port.Listening))
			r.ProxyURL = gperr.Collect(&errs, nettypes.ParseURL, fmt.Sprintf("%s://%s:%d", rScheme, r.Host, r.Port.Proxy))
		}

		// should exclude, we don't care the scheme here.
		r.ProxyURL = gperr.Collect(&errs, nettypes.ParseURL, fmt.Sprintf("%s://%s:%d", r.Scheme, r.Host, r.Port.Proxy))
	}

	if !r.UseHealthCheck() && (r.UseLoadBalance() || r.UseIdleWatcher()) {
		errs.Adds("cannot disable healthcheck when loadbalancer or idle watcher is enabled")
	}

	if errs.HasError() {
		return errs.Error()
	}

	switch r.Scheme {
	case route.SchemeFileServer:
		impl, err = NewFileServer(r)
	case route.SchemeHTTP, route.SchemeHTTPS, route.SchemeH2C:
		impl, err = NewReverseProxyRoute(r)
	case route.SchemeTCP, route.SchemeUDP:
		impl, err = NewStreamRoute(r)
	default:
		panic(fmt.Errorf("unexpected scheme %s for alias %s", r.Scheme, r.Alias))
	}

	if err != nil {
		return err
	}

	r.impl = impl
	r.Excluded = r.ShouldExclude()
	if r.Excluded {
		r.ExcludedReason = r.findExcludedReason()
	}
	return nil
}

func (r *Route) validateRules() error {
	// FIXME: hardcoded here as a workaround
	// there's already a label "proxy.#1.rule_file=embed://webui.yml"
	// but it's not working as expected sometimes.
	// TODO: investigate why it's not working and fix it.
	if cont := r.ContainerInfo(); cont != nil {
		if cont.Image.Name == "godoxy-frontend" {
			rules, ok := rulepresets.GetRulePreset("webui.yml")
			if !ok {
				return errors.New("rule preset `webui.yml` not found")
			}
			r.Rules = rules
		}
		return nil
	}

	if r.RuleFile != "" && len(r.Rules) > 0 {
		return errors.New("`rule_file` and `rules` cannot be used together")
	} else if r.RuleFile != "" {
		src, err := url.Parse(r.RuleFile)
		if err != nil {
			return fmt.Errorf("failed to parse rule file url %q: %w", r.RuleFile, err)
		}
		switch src.Scheme {
		case "embed": // embed://<preset_file_name>
			rules, ok := rulepresets.GetRulePreset(src.Host)
			if !ok {
				return fmt.Errorf("rule preset %q not found", src.Host)
			} else {
				r.Rules = rules
			}
		case "file", "":
			content, err := os.ReadFile(src.Path)
			if err != nil {
				return fmt.Errorf("failed to read rule file %q: %w", src.Path, err)
			} else {
				_, err = serialization.ConvertString(string(content), reflect.ValueOf(&r.Rules))
				if err != nil {
					return fmt.Errorf("failed to unmarshal rule file %q: %w", src.Path, err)
				}
			}
		default:
			return fmt.Errorf("unsupported rule file scheme %q", src.Scheme)
		}
	}
	return nil
}

func (r *Route) Impl() types.Route {
	return r.impl
}

func (r *Route) Task() *task.Task {
	return r.task
}

func (r *Route) Start(parent task.Parent) gperr.Error {
	r.onceStart.Do(func() {
		r.startErr.Set(r.start(parent))
	})
	return r.startErr.Get()
}

func (r *Route) start(parent task.Parent) gperr.Error {
	if r.impl == nil { // should not happen
		return gperr.New("route not initialized")
	}
	defer close(r.started)

	// skip checking for excluded routes
	excluded := r.ShouldExclude()
	if !excluded {
		if err := checkExists(r); err != nil {
			return err
		}
	}

	if cont := r.ContainerInfo(); cont != nil {
		docker.SetDockerCfgByContainerID(cont.ContainerID, cont.DockerCfg)
	}

	if !excluded {
		if err := r.impl.Start(parent); err != nil {
			return err
		}
	} else {
		r.task = parent.Subtask("excluded."+r.Name(), true)
		routes.Excluded.Add(r.impl)
		r.task.OnCancel("remove_route_from_excluded", func() {
			routes.Excluded.Del(r.impl)
		})
		if r.UseHealthCheck() {
			r.HealthMon = monitor.NewMonitor(r.impl)
			err := r.HealthMon.Start(r.task)
			return err
		}
	}
	return nil
}

func (r *Route) Finish(reason any) {
	if cont := r.ContainerInfo(); cont != nil {
		docker.DeleteDockerCfgByContainerID(cont.ContainerID)
	}
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

func (r *Route) GetProvider() types.RouteProvider {
	return r.provider
}

func (r *Route) SetProvider(p types.RouteProvider) {
	r.provider = p
	r.Provider = p.ShortName()
}

func (r *Route) ProviderName() string {
	return r.Provider
}

func (r *Route) TargetURL() *nettypes.URL {
	return r.ProxyURL
}

func (r *Route) References() []string {
	if r.Container != nil {
		if r.Container.ContainerName != r.Alias {
			return []string{r.Container.ContainerName, r.Alias, r.Container.Image.Name, r.Container.Image.Author}
		}
		return []string{r.Container.Image.Name, r.Alias, r.Container.Image.Author}
	}
	return []string{r.Alias}
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

func (r *Route) Type() route.RouteType {
	switch r.Scheme {
	case route.SchemeHTTP, route.SchemeHTTPS, route.SchemeFileServer:
		return route.RouteTypeHTTP
	case route.SchemeTCP, route.SchemeUDP:
		return route.RouteTypeStream
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

func (r *Route) HealthMonitor() types.HealthMonitor {
	return r.HealthMon
}

func (r *Route) SetHealthMonitor(m types.HealthMonitor) {
	if r.HealthMon != nil && r.HealthMon != m {
		r.HealthMon.Finish("health monitor replaced")
	}
	r.HealthMon = m
}

func (r *Route) IdlewatcherConfig() *types.IdlewatcherConfig {
	return r.Idlewatcher
}

func (r *Route) HealthCheckConfig() types.HealthCheckConfig {
	return r.HealthCheck
}

func (r *Route) LoadBalanceConfig() *types.LoadBalancerConfig {
	return r.LoadBalance
}

func (r *Route) HomepageItem() homepage.Item {
	containerID := ""
	if r.Container != nil {
		containerID = r.Container.ContainerID
	}
	return homepage.Item{
		Alias:       r.Alias,
		Provider:    r.Provider,
		ItemConfig:  *r.Homepage,
		ContainerID: containerID,
	}.GetOverride()
}

func (r *Route) DisplayName() string {
	if r.Homepage == nil { // should only happen in tests, Validate() should initialize it
		return r.Alias
	}
	return r.Homepage.Name
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
	case *ReveseProxyRoute:
		or = v.Route
	case *FileServer:
		or = v.Route
	case *StreamRoute:
		or = v.Route
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

	// Prefer explicit (non-docker) over docker auto-discovered
	if (r.Container == nil) != (or.Container == nil) {
		return r.Container == nil
	}

	// Default: allow replacement
	return true
}

func (r *Route) ContainerInfo() *types.Container {
	return r.Container
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
	return r.findExcludedReason() != ExcludedReasonNone
}

type ExcludedReason uint8

const (
	ExcludedReasonNone ExcludedReason = iota
	ExcludedReasonError
	ExcludedReasonManual
	ExcludedReasonNoPortContainer
	ExcludedReasonNoPortSpecified
	ExcludedReasonBlacklisted
	ExcludedReasonBuildx
	ExcludedReasonOld
)

func (re ExcludedReason) String() string {
	switch re {
	case ExcludedReasonNone:
		return ""
	case ExcludedReasonError:
		return "Error"
	case ExcludedReasonManual:
		return "Manual exclusion"
	case ExcludedReasonNoPortContainer:
		return "No port exposed in container"
	case ExcludedReasonNoPortSpecified:
		return "No port specified"
	case ExcludedReasonBlacklisted:
		return "Blacklisted (backend service or database)"
	case ExcludedReasonBuildx:
		return "Buildx"
	case ExcludedReasonOld:
		return "Container renaming intermediate state"
	default:
		return "Unknown"
	}
}

func (re ExcludedReason) MarshalJSON() ([]byte, error) {
	return strconv.AppendQuote(nil, re.String()), nil
}

// no need to unmarshal json because we don't store this

func (r *Route) findExcludedReason() ExcludedReason {
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
		case r.IsZeroPort() && !r.UseIdleWatcher():
			return ExcludedReasonNoPortContainer
		case !r.Container.IsExplicit && docker.IsBlacklisted(r.Container):
			return ExcludedReasonBlacklisted
		case strings.HasPrefix(r.Container.ContainerName, "buildx_"):
			return ExcludedReasonBuildx
		}
	} else if r.IsZeroPort() && r.Scheme != route.SchemeFileServer {
		return ExcludedReasonNoPortSpecified
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
		switch {
		case r.Container.Image.Name == "godoxy-agent":
			return false
		case !r.Container.Running && !r.UseIdleWatcher():
			return false
		case strings.HasPrefix(r.Container.ContainerName, "buildx_"):
			return false
		}
	}
	return !r.HealthCheck.Disable
}

func (r *Route) UseAccessLog() bool {
	return r.AccessLog != nil
}

func (r *Route) Finalize() {
	r.Alias = strings.ToLower(strings.TrimSpace(r.Alias))
	r.Host = strings.ToLower(strings.TrimSpace(r.Host))

	isDocker := r.Container != nil
	cont := r.Container

	if r.Host == "" {
		switch {
		case !isDocker:
			r.Host = "localhost"
		case cont.PrivateHostname != "":
			r.Host = cont.PrivateHostname
		case cont.PublicHostname != "":
			r.Host = cont.PublicHostname
		}
	}

	lp, pp := r.Port.Listening, r.Port.Proxy

	if isDocker {
		scheme, port, ok := getSchemePortByImageName(cont.Image.Name)
		if ok {
			if r.Scheme == route.SchemeNone {
				r.Scheme = scheme
			}
			if pp == 0 {
				pp = port
			}
		}
	}

	if scheme, port, ok := getSchemePortByAlias(r.Alias); ok {
		if r.Scheme == route.SchemeNone {
			r.Scheme = scheme
		}
		if pp == 0 {
			pp = port
		}
	}

	if pp == 0 {
		switch {
		case isDocker:
			if cont.IsHostNetworkMode {
				pp = preferredPort(cont.PublicPortMapping)
			} else {
				pp = preferredPort(cont.PrivatePortMapping)
			}
		case r.Scheme == route.SchemeHTTPS:
			pp = 443
		default:
			pp = 80
		}
	}

	if isDocker {
		if r.Scheme == route.SchemeNone {
			for _, p := range cont.PublicPortMapping {
				if int(p.PrivatePort) == pp && p.Type == "udp" {
					r.Scheme = route.SchemeUDP
					break
				}
			}
		}
		// replace private port with public port if using public IP.
		if r.Host == cont.PublicHostname {
			if p, ok := cont.PrivatePortMapping[pp]; ok {
				pp = int(p.PublicPort)
			}
		} else {
			// replace public port with private port if using private IP.
			if p, ok := cont.PublicPortMapping[pp]; ok {
				pp = int(p.PrivatePort)
			}
		}
	}

	if r.Scheme == route.SchemeNone {
		switch {
		case lp != 0:
			r.Scheme = route.SchemeTCP
		case pp%1000 == 443:
			r.Scheme = route.SchemeHTTPS
		default: // assume its http
			r.Scheme = route.SchemeHTTP
		}
	}

	r.Port.Listening, r.Port.Proxy = lp, pp

	workingState := config.WorkingState.Load()
	if workingState == nil {
		if common.IsTest { // in tests, working state might be nil
			return
		}
		panic("bug: working state is nil")
	}

	r.HealthCheck.ApplyDefaults(config.WorkingState.Load().Value().Defaults.HealthCheck)
}

func (r *Route) FinalizeHomepageConfig() {
	if r.Alias == "" {
		panic("alias is empty")
	}

	isDocker := r.Container != nil

	if r.Homepage == nil {
		r.Homepage = &homepage.ItemConfig{
			Show: true,
		}
	}

	if r.ShouldExclude() && isDocker {
		r.Homepage.Show = false
		r.Homepage.Name = r.Container.ContainerName // still show container name in metrics page
		return
	}

	hp := r.Homepage
	refs := r.References()
	for _, ref := range refs {
		meta, ok := homepage.GetHomepageMeta(ref)
		if ok {
			if hp.Name == "" {
				hp.Name = meta.DisplayName
			}
			if hp.Category == "" {
				hp.Category = meta.Tag
			}
			break
		}
	}

	if hp.Name == "" {
		hp.Name = strutils.Title(
			strings.ReplaceAll(
				strings.ReplaceAll(refs[0], "-", " "),
				"_", " ",
			),
		)
	}

	if hp.Category == "" {
		if homepagecfg.ActiveConfig.Load().UseDefaultCategories {
			for _, ref := range refs {
				if category, ok := homepage.PredefinedCategories[ref]; ok {
					hp.Category = category
					break
				}
			}
		}

		if hp.Category == "" {
			switch {
			case r.UseLoadBalance():
				hp.Category = "Load-balanced"
			case isDocker:
				hp.Category = "Docker"
			default:
				hp.Category = "Others"
			}
		}
	}
}

var preferredPortOrder = []int{
	80,
	8080,
	3000,
	8000,
	443,
	8443,
}

func preferredPort(portMapping types.PortMapping) (res int) {
	for _, port := range preferredPortOrder {
		if _, ok := portMapping[port]; ok {
			return port
		}
	}
	// fallback to lowest port
	cmp := (uint16)(65535)
	for port, v := range portMapping {
		if v.PrivatePort < cmp {
			cmp = v.PrivatePort
			res = port
		}
	}
	return res
}
