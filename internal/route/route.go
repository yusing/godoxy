package route

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/internal/docker"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/homepage"
	idlewatcher "github.com/yusing/go-proxy/internal/idlewatcher/types"
	netutils "github.com/yusing/go-proxy/internal/net"
	nettypes "github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/proxmox"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils/strutils"
	"github.com/yusing/go-proxy/internal/watcher/health"

	"github.com/yusing/go-proxy/internal/common"
	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/logging/accesslog"
	loadbalance "github.com/yusing/go-proxy/internal/net/gphttp/loadbalancer/types"
	"github.com/yusing/go-proxy/internal/route/routes"
	"github.com/yusing/go-proxy/internal/route/rules"
	route "github.com/yusing/go-proxy/internal/route/types"
	"github.com/yusing/go-proxy/internal/utils"
)

type (
	Route struct {
		_ utils.NoCopy

		Alias  string       `json:"alias"`
		Scheme route.Scheme `json:"scheme,omitempty"`
		Host   string       `json:"host,omitempty"`
		Port   route.Port   `json:"port"`
		Root   string       `json:"root,omitempty"`

		route.HTTPConfig
		PathPatterns []string                       `json:"path_patterns,omitempty"`
		Rules        rules.Rules                    `json:"rules,omitempty" validate:"omitempty,unique=Name"`
		HealthCheck  *health.HealthCheckConfig      `json:"healthcheck,omitempty"`
		LoadBalance  *loadbalance.Config            `json:"load_balance,omitempty"`
		Middlewares  map[string]docker.LabelMap     `json:"middlewares,omitempty"`
		Homepage     *homepage.ItemConfig           `json:"homepage,omitempty"`
		AccessLog    *accesslog.RequestLoggerConfig `json:"access_log,omitempty"`
		Agent        string                         `json:"agent,omitempty"`

		Idlewatcher *idlewatcher.Config  `json:"idlewatcher,omitempty"`
		HealthMon   health.HealthMonitor `json:"health,omitempty"`

		Metadata `deserialize:"-"`
	}

	Metadata struct {
		/* Docker only */
		Container *docker.Container `json:"container,omitempty"`

		Provider string `json:"provider,omitempty"` // for backward compatibility

		// private fields
		LisURL   *nettypes.URL `json:"lurl,omitempty"`
		ProxyURL *nettypes.URL `json:"purl,omitempty"`

		Excluded *bool `json:"excluded"`

		impl routes.Route
		task *task.Task

		isValidated bool
		lastError   gperr.Error
		provider    routes.Provider

		agent *agent.AgentConfig

		started chan struct{}
		once    sync.Once
	}
	Routes map[string]*Route
)

const DefaultHost = "localhost"

func (r Routes) Contains(alias string) bool {
	_, ok := r[alias]
	return ok
}

func (r *Route) Validate() gperr.Error {
	if r.isValidated {
		return r.lastError
	}
	r.isValidated = true

	if r.Agent != "" {
		if r.Container != nil {
			return gperr.Errorf("specifying agent is not allowed for docker container routes")
		}
		var ok bool
		// by agent address
		r.agent, ok = agent.GetAgent(r.Agent)
		if !ok {
			// fallback to get agent by name
			r.agent, ok = agent.GetAgentByName(r.Agent)
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

	errs := gperr.NewBuilder("entry validation failed")

	var impl routes.Route
	var err gperr.Error

	switch r.Scheme {
	case route.SchemeFileServer:
		r.ProxyURL = gperr.Collect(errs, nettypes.ParseURL, "file://"+r.Root)
		r.Host = ""
		r.Port.Proxy = 0
	case route.SchemeHTTP, route.SchemeHTTPS:
		if r.Port.Listening != 0 {
			errs.Addf("unexpected listening port for %s scheme", r.Scheme)
		}
		r.ProxyURL = gperr.Collect(errs, nettypes.ParseURL, fmt.Sprintf("%s://%s:%d", r.Scheme, r.Host, r.Port.Proxy))
	case route.SchemeTCP, route.SchemeUDP:
		if !r.ShouldExclude() {
			r.LisURL = gperr.Collect(errs, nettypes.ParseURL, fmt.Sprintf("%s://:%d", r.Scheme, r.Port.Listening))
		}
		r.ProxyURL = gperr.Collect(errs, nettypes.ParseURL, fmt.Sprintf("%s://%s:%d", r.Scheme, r.Host, r.Port.Proxy))
	}

	if !r.UseHealthCheck() && (r.UseLoadBalance() || r.UseIdleWatcher()) {
		errs.Adds("cannot disable healthcheck when loadbalancer or idle watcher is enabled")
	}

	if errs.HasError() {
		r.lastError = errs.Error()
		return errs.Error()
	}

	switch r.Scheme {
	case route.SchemeFileServer:
		impl, err = NewFileServer(r)
	case route.SchemeHTTP, route.SchemeHTTPS:
		impl, err = NewReverseProxyRoute(r)
	case route.SchemeTCP, route.SchemeUDP:
		impl, err = NewStreamRoute(r)
	default:
		panic(fmt.Errorf("unexpected scheme %s for alias %s", r.Scheme, r.Alias))
	}

	if err != nil {
		r.lastError = err
		return err
	}

	r.impl = impl
	excluded := r.ShouldExclude()
	r.Excluded = &excluded
	return nil
}

func (r *Route) Impl() routes.Route {
	return r.impl
}

func (r *Route) Task() *task.Task {
	return r.task
}

func (r *Route) Start(parent task.Parent) (err gperr.Error) {
	r.once.Do(func() {
		err = r.start(parent)
	})
	return
}

func (r *Route) start(parent task.Parent) gperr.Error {
	if r.impl == nil { // should not happen
		return gperr.New("route not initialized")
	}
	defer close(r.started)

	if err := r.impl.Start(parent); err != nil {
		return err
	}

	if conflict, added := routes.All.AddIfNotExists(r.impl); !added {
		err := gperr.Errorf("route %s already exists: from %s and %s", r.Alias, r.ProviderName(), conflict.ProviderName())
		r.task.FinishAndWait(err)
		return err
	} else {
		// reference here because r.impl will be nil after Finish() is called.
		impl := r.impl
		r.task.OnCancel("remove_routes_from_all", func() {
			routes.All.Del(impl)
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

func (r *Route) GetProvider() routes.Provider {
	return r.provider
}

func (r *Route) SetProvider(p routes.Provider) {
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

func (r *Route) GetAgent() *agent.AgentConfig {
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

func (r *Route) HealthCheckConfig() *health.HealthCheckConfig {
	return r.HealthCheck
}

func (r *Route) LoadBalanceConfig() *loadbalance.Config {
	return r.LoadBalance
}

func (r *Route) HomepageConfig() *homepage.ItemConfig {
	return r.Homepage.GetOverride(r.Alias)
}

func (r *Route) HomepageItem() *homepage.Item {
	return &homepage.Item{
		Alias:      r.Alias,
		Provider:   r.Provider,
		ItemConfig: r.HomepageConfig(),
	}
}

func (r *Route) ContainerInfo() *docker.Container {
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
	if r.lastError != nil {
		return true
	}
	if r.Excluded != nil {
		return *r.Excluded
	}
	if r.Container != nil {
		switch {
		case r.Container.IsExcluded:
			return true
		case r.IsZeroPort() && !r.UseIdleWatcher():
			return true
		case !r.Container.IsExplicit && r.Container.IsBlacklisted():
			return true
		case strings.HasPrefix(r.Container.ContainerName, "buildx_"):
			return true
		}
	} else if r.IsZeroPort() && r.Scheme != route.SchemeFileServer {
		return true
	}
	if strings.HasSuffix(r.Alias, "-old") {
		return true
	}
	return false
}

func (r *Route) UseLoadBalance() bool {
	return r.LoadBalance != nil && r.LoadBalance.Link != ""
}

func (r *Route) UseIdleWatcher() bool {
	return r.Idlewatcher != nil && r.Idlewatcher.IdleTimeout > 0
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
			if r.Scheme == "" {
				r.Scheme = route.Scheme(scheme)
			}
			if pp == 0 {
				pp = port
			}
		}
	}

	if scheme, port, ok := getSchemePortByAlias(r.Alias); ok {
		if r.Scheme == "" {
			r.Scheme = route.Scheme(scheme)
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
		case r.Scheme == "https":
			pp = 443
		default:
			pp = 80
		}
	}

	if isDocker {
		if r.Scheme == "" {
			for _, p := range cont.PublicPortMapping {
				if int(p.PrivatePort) == pp && p.Type == "udp" {
					r.Scheme = "udp"
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

	if r.Scheme == "" {
		switch {
		case lp != 0:
			r.Scheme = "tcp"
		case pp%1000 == 443:
			r.Scheme = "https"
		default: // assume its http
			r.Scheme = "http"
		}
	}

	r.Port.Listening, r.Port.Proxy = lp, pp

	if r.HealthCheck == nil {
		r.HealthCheck = health.DefaultHealthConfig()
	}

	if !r.HealthCheck.Disable {
		if r.HealthCheck.Interval == 0 {
			r.HealthCheck.Interval = common.HealthCheckIntervalDefault
		}
		if r.HealthCheck.Timeout == 0 {
			r.HealthCheck.Timeout = common.HealthCheckTimeoutDefault
		}
	}
}

func (r *Route) FinalizeHomepageConfig() {
	if r.Alias == "" {
		panic("alias is empty")
	}

	isDocker := r.Container != nil

	if r.Homepage == nil {
		r.Homepage = &homepage.ItemConfig{Show: true}
	}
	r.Homepage = r.Homepage.GetOverride(r.Alias)

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
		if config.GetInstance().Value().Homepage.UseDefaultCategories {
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

func preferredPort(portMapping map[int]container.Port) (res int) {
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
	return
}
