package route

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/yusing/go-proxy/agent/pkg/agent"

	"github.com/yusing/go-proxy/internal/docker"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/homepage"
	idlewatcher "github.com/yusing/go-proxy/internal/idlewatcher/types"
	"github.com/yusing/go-proxy/internal/logging"
	gpnet "github.com/yusing/go-proxy/internal/net"
	"github.com/yusing/go-proxy/internal/proxmox"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils/strutils"
	"github.com/yusing/go-proxy/internal/watcher/health"

	"github.com/yusing/go-proxy/internal/common"
	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/net/gphttp/accesslog"
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
		Port   route.Port   `json:"port,omitempty"`
		Root   string       `json:"root,omitempty"`

		route.HTTPConfig
		PathPatterns []string                   `json:"path_patterns,omitempty"`
		Rules        rules.Rules                `json:"rules,omitempty" validate:"omitempty,unique=Name"`
		HealthCheck  *health.HealthCheckConfig  `json:"healthcheck,omitempty"`
		LoadBalance  *loadbalance.Config        `json:"load_balance,omitempty"`
		Middlewares  map[string]docker.LabelMap `json:"middlewares,omitempty"`
		Homepage     *homepage.ItemConfig       `json:"homepage,omitempty"`
		AccessLog    *accesslog.Config          `json:"access_log,omitempty"`

		Idlewatcher *idlewatcher.Config `json:"idlewatcher,omitempty"`

		Metadata `deserialize:"-"`
	}

	Metadata struct {
		/* Docker only */
		Container *docker.Container `json:"container,omitempty"`
		Provider  string            `json:"provider,omitempty"`

		// private fields
		LisURL   *url.URL `json:"lurl,omitempty"`
		ProxyURL *url.URL `json:"purl,omitempty"`

		impl routes.Route
	}
	Routes map[string]*Route
)

const DefaultHost = "localhost"

func (r Routes) Contains(alias string) bool {
	_, ok := r[alias]
	return ok
}

func (r *Route) Validate() (err gperr.Error) {
	r.Finalize()

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

			l := logging.With().Str("container", containerName).Logger()

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
			for _, ip := range ips {
				if ok, _ := gpnet.PingWithTCPFallback(ctx, ip, r.Port.Proxy); ok {
					r.Host = ip.String()
					l.Info().Msgf("using ip %s", r.Host)
					break
				}
			}
			if r.Host == DefaultHost {
				return gperr.Multiline().
					Addf("no reachable ip addresses found, tried %d IPs", len(ips)).
					AddLines(ips).
					Subject(containerName)
			}
		}
	}

	// return error if route is localhost:<godoxy_port>
	switch r.Host {
	case "localhost", "127.0.0.1":
		switch r.Port.Proxy {
		case common.ProxyHTTPPort, common.ProxyHTTPSPort, common.APIHTTPPort:
			if r.Scheme.IsReverseProxy() || r.Scheme == route.SchemeTCP {
				return gperr.Errorf("localhost:%d is reserved for godoxy", r.Port.Proxy)
			}
		}
	}

	errs := gperr.NewBuilder("entry validation failed")

	if r.Scheme == route.SchemeFileServer {
		r.impl, err = NewFileServer(r)
		if err != nil {
			errs.Add(err)
		}
		r.ProxyURL = gperr.Collect(errs, url.Parse, "file://"+r.Root)
		r.Host = ""
		r.Port.Proxy = 0
	} else {
		switch r.Scheme {
		case route.SchemeHTTP, route.SchemeHTTPS:
			if r.Port.Listening != 0 {
				errs.Addf("unexpected listening port for %s scheme", r.Scheme)
			}
		case route.SchemeTCP, route.SchemeUDP:
			r.LisURL = gperr.Collect(errs, url.Parse, fmt.Sprintf("%s://:%d", r.Scheme, r.Port.Listening))
		}
		r.ProxyURL = gperr.Collect(errs, url.Parse, fmt.Sprintf("%s://%s:%d", r.Scheme, r.Host, r.Port.Proxy))
	}

	if !r.UseHealthCheck() && (r.UseLoadBalance() || r.UseIdleWatcher()) {
		errs.Adds("cannot disable healthcheck when loadbalancer or idle watcher is enabled")
	}

	if errs.HasError() {
		return errs.Error()
	}

	switch r.Scheme {
	case route.SchemeFileServer:
		r.impl, err = NewFileServer(r)
	case route.SchemeHTTP, route.SchemeHTTPS:
		r.impl, err = NewReverseProxyRoute(r)
	case route.SchemeTCP, route.SchemeUDP:
		r.impl, err = NewStreamRoute(r)
	default:
		panic(fmt.Errorf("unexpected scheme %s for alias %s", r.Scheme, r.Alias))
	}

	return err
}

func (r *Route) Start(parent task.Parent) (err gperr.Error) {
	if r.impl == nil {
		return gperr.New("route not initialized")
	}

	return r.impl.Start(parent)
}

func (r *Route) Finish(reason any) {
	if r.impl == nil {
		return
	}
	r.impl.Finish(reason)
	r.impl = nil
}

func (r *Route) Started() bool {
	return r.impl != nil
}

func (r *Route) Reference() string {
	if r.Container != nil {
		return r.Container.Image.Name
	}
	return r.Alias
}

func (r *Route) ProviderName() string {
	return r.Provider
}

// Name implements pool.Object.
func (r *Route) Name() string {
	return r.Alias
}

// Key implements pool.Object.
func (r *Route) Key() string {
	return r.Alias
}

func (r *Route) String() string {
	return r.Alias
}

func (r *Route) TargetURL() *url.URL {
	return r.ProxyURL
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

func (r *Route) Agent() *agent.AgentConfig {
	if r.Container == nil {
		return nil
	}
	return r.Container.Agent
}

func (r *Route) IsAgent() bool {
	return r.Container != nil && r.Container.Agent != nil
}

func (r *Route) HealthMonitor() health.HealthMonitor {
	return r.impl.HealthMonitor()
}

func (r *Route) IdlewatcherConfig() *idlewatcher.Config {
	cont := r.Container
	if cont != nil && cont.IdlewatcherConfig != nil {
		return cont.IdlewatcherConfig
	}
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
	} else if r.IsZeroPort() {
		return true
	}
	if strings.HasPrefix(r.Alias, "x-") ||
		strings.HasSuffix(r.Alias, "-old") {
		return true
	}
	return false
}

func (r *Route) UseLoadBalance() bool {
	return r.LoadBalance != nil && r.LoadBalance.Link != ""
}

func (r *Route) UseIdleWatcher() bool {
	cfg := r.IdlewatcherConfig()
	return cfg != nil && cfg.IdleTimeout > 0
}

func (r *Route) UseHealthCheck() bool {
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
			r.Host = DefaultHost
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
			pp = lowestPort(cont.PrivatePortMapping)
			if pp == 0 {
				pp = lowestPort(cont.PublicPortMapping)
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
				if p.PrivatePort == uint16(pp) && p.Type == "udp" {
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

	if r.LoadBalance != nil && r.LoadBalance.Link == "" {
		r.LoadBalance = nil
	}
}

func (r *Route) FinalizeHomepageConfig() {
	if r.Alias == "" {
		panic("alias is empty")
	}

	isDocker := r.IsDocker()

	if r.Homepage == nil {
		r.Homepage = &homepage.ItemConfig{Show: true}
	}
	r.Homepage = r.Homepage.GetOverride(r.Alias)

	hp := r.Homepage
	ref := r.Reference()

	if hp.Name == "" {
		displayName, ok := homepage.GetDisplayName(ref)
		if ok {
			hp.Name = displayName
		} else {
			hp.Name = strutils.Title(
				strings.ReplaceAll(
					strings.ReplaceAll(r.Alias, "-", " "),
					"_", " ",
				),
			)
		}
	}

	if hp.Category == "" {
		if config.GetInstance().Value().Homepage.UseDefaultCategories {
			if category, ok := homepage.PredefinedCategories[ref]; ok {
				hp.Category = category
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

func lowestPort(ports docker.PortMapping) (res int) {
	cmp := (uint16)(65535)
	for port, v := range ports {
		if v.PrivatePort < cmp {
			cmp = v.PrivatePort
			res = port
		}
	}
	return
}
