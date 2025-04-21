package route

import (
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/internal"
	"github.com/yusing/go-proxy/internal/docker"
	idlewatcher "github.com/yusing/go-proxy/internal/docker/idlewatcher/types"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/homepage"
	net "github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils/strutils"
	"github.com/yusing/go-proxy/internal/watcher/health"

	"github.com/yusing/go-proxy/internal/common"
	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/net/gphttp/accesslog"
	loadbalance "github.com/yusing/go-proxy/internal/net/gphttp/loadbalancer/types"
	"github.com/yusing/go-proxy/internal/route/rules"
	"github.com/yusing/go-proxy/internal/route/types"
	"github.com/yusing/go-proxy/internal/utils"
)

type (
	Route struct {
		_ utils.NoCopy

		Alias  string       `json:"alias"`
		Scheme types.Scheme `json:"scheme,omitempty"`
		Host   string       `json:"host,omitempty"`
		Port   types.Port   `json:"port,omitempty"`
		Root   string       `json:"root,omitempty"`

		types.HTTPConfig
		PathPatterns []string                   `json:"path_patterns,omitempty"`
		Rules        rules.Rules                `json:"rules,omitempty" validate:"omitempty,unique=Name"`
		HealthCheck  *health.HealthCheckConfig  `json:"healthcheck,omitempty"`
		LoadBalance  *loadbalance.Config        `json:"load_balance,omitempty"`
		Middlewares  map[string]docker.LabelMap `json:"middlewares,omitempty"`
		Homepage     *homepage.ItemConfig       `json:"homepage,omitempty"`
		AccessLog    *accesslog.Config          `json:"access_log,omitempty"`

		Metadata `deserialize:"-"`
	}

	Metadata struct {
		/* Docker only */
		Container *docker.Container `json:"container,omitempty"`
		Provider  string            `json:"provider,omitempty"`

		// private fields
		LisURL      *net.URL            `json:"lurl,omitempty"`
		ProxyURL    *net.URL            `json:"purl,omitempty"`
		Idlewatcher *idlewatcher.Config `json:"idlewatcher,omitempty"`

		impl        types.Route
		isValidated bool
	}
	Routes map[string]*Route
)

func (r Routes) Contains(alias string) bool {
	_, ok := r[alias]
	return ok
}

func (r *Route) Validate() (err gperr.Error) {
	if r.isValidated {
		return nil
	}
	r.isValidated = true
	r.Finalize()

	// return error if route is localhost:<godoxy_port>
	switch r.Host {
	case "localhost", "127.0.0.1":
		switch r.Port.Proxy {
		case common.ProxyHTTPPort, common.ProxyHTTPSPort, common.APIHTTPPort:
			if r.Scheme.IsReverseProxy() || r.Scheme == types.SchemeTCP {
				return gperr.Errorf("localhost:%d is reserved for godoxy", r.Port.Proxy)
			}
		}
	}

	errs := gperr.NewBuilder("entry validation failed")

	switch r.Scheme {
	case types.SchemeFileServer:
		r.impl, err = NewFileServer(r)
		if err != nil {
			errs.Add(err)
		}
	case types.SchemeHTTP, types.SchemeHTTPS:
		if r.Port.Listening != 0 {
			errs.Addf("unexpected listening port for %s scheme", r.Scheme)
		}
		fallthrough
	case types.SchemeTCP, types.SchemeUDP:
		r.LisURL = gperr.Collect(errs, net.ParseURL, fmt.Sprintf("%s://:%d", r.Scheme, r.Port.Listening))
		fallthrough
	default:
		if r.LoadBalance != nil && r.LoadBalance.Link == "" {
			r.LoadBalance = nil
		}
		r.ProxyURL = gperr.Collect(errs, net.ParseURL, fmt.Sprintf("%s://%s:%d", r.Scheme, r.Host, r.Port.Proxy))
		r.Idlewatcher = gperr.Collect(errs, idlewatcher.ValidateConfig, r.Container)
	}

	if !r.UseHealthCheck() && (r.UseLoadBalance() || r.UseIdleWatcher()) {
		errs.Adds("cannot disable healthcheck when loadbalancer or idle watcher is enabled")
	}

	if errs.HasError() {
		return errs.Error()
	}

	var impl types.Route
	switch r.Scheme {
	case types.SchemeFileServer:
		impl, err = NewFileServer(r)
	case types.SchemeHTTP, types.SchemeHTTPS:
		impl, err = NewReverseProxyRoute(r)
	case types.SchemeTCP, types.SchemeUDP:
		impl, err = NewStreamRoute(r)
	default:
		panic(fmt.Errorf("unexpected scheme %s for alias %s", r.Scheme, r.Alias))
	}

	if err != nil {
		return err
	}

	r.impl = impl
	return nil
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

func (r *Route) ProviderName() string {
	return r.Provider
}

func (r *Route) TargetName() string {
	return r.Alias
}

func (r *Route) TargetURL() *net.URL {
	return r.ProxyURL
}

func (r *Route) Type() types.RouteType {
	switch r.Scheme {
	case types.SchemeHTTP, types.SchemeHTTPS, types.SchemeFileServer:
		return types.RouteTypeHTTP
	case types.SchemeTCP, types.SchemeUDP:
		return types.RouteTypeStream
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
	return r.Idlewatcher != nil && r.Idlewatcher.IdleTimeout > 0
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
				r.Scheme = types.Scheme(scheme)
			}
			if pp == 0 {
				pp = port
			}
		}
	}

	if scheme, port, ok := getSchemePortByAlias(r.Alias); ok {
		if r.Scheme == "" {
			r.Scheme = types.Scheme(scheme)
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

	if isDocker && cont.IdleTimeout != "" {
		if cont.WakeTimeout == "" {
			cont.WakeTimeout = common.WakeTimeoutDefault
		}
		if cont.StopTimeout == "" {
			cont.StopTimeout = common.StopTimeoutDefault
		}
		if cont.StopMethod == "" {
			cont.StopMethod = common.StopMethodDefault
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

	hp := r.Homepage

	var key string
	if hp.Name == "" {
		if r.Container != nil {
			key = r.Container.Image.Name
		} else {
			key = r.Alias
		}
		displayName, ok := internal.GetDisplayName(key)
		if ok {
			hp.Name = displayName
		} else {
			hp.Name = strutils.Title(
				strings.ReplaceAll(
					strings.ReplaceAll(key, "-", " "),
					"_", " ",
				),
			)
		}
	}

	if hp.Category == "" {
		if config.GetInstance().Value().Homepage.UseDefaultCategories {
			if isDocker {
				key = r.Container.Image.Name
			} else {
				key = strings.ToLower(r.Alias)
			}
			if category, ok := homepage.PredefinedCategories[key]; ok {
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

func lowestPort(ports map[int]container.Port) (res int) {
	cmp := (uint16)(65535)
	for port, v := range ports {
		if v.PrivatePort < cmp {
			cmp = v.PrivatePort
			res = port
		}
	}
	return
}
