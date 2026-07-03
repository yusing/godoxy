package routevalidate

import (
	"strings"

	"github.com/yusing/godoxy/internal/common"
	config "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/homepage"
	iconlist "github.com/yusing/godoxy/internal/homepage/icons/list"
	homepagecfg "github.com/yusing/godoxy/internal/homepage/types"
	"github.com/yusing/godoxy/internal/route"
	strutils "github.com/yusing/goutils/strings"
)

// finalize fill in missing fields with proper values.
func finalize(r *route.Route) {
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

	switch r.Scheme {
	case route.SchemeTCP, route.SchemeUDP:
		if r.Bind == "" {
			r.Bind = "0.0.0.0"
		}
	}

	r.Port.Listening, r.Port.Proxy = lp, pp
	r.CanResolveDockerProxyPort = canResolveDockerProxyPort(r)
	r.CheckedDockerProxyPort = true

	workingState := config.WorkingState.Load()
	if workingState == nil {
		if common.IsTest { // in tests, working state might be nil
			return
		}
		panic("bug: working state is nil")
	}

	// TODO: default value from context
	r.HealthCheck.ApplyDefaults(workingState.Value().Defaults.HealthCheck)

	finalizeHomepageConfig(r)
}

func finalizeHomepageConfig(r *route.Route) {
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
		meta, ok := iconlist.GetMetadata(ref)
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

func canResolveDockerProxyPort(r *route.Route) bool {
	if r.Port.Proxy != 0 {
		return true
	}
	if r.Container == nil {
		return false
	}
	if r.Container.Image != nil {
		if _, port, ok := getSchemePortByImageName(r.Container.Image.Name); ok && port != 0 {
			return true
		}
	}
	if _, port, ok := getSchemePortByAlias(r.Alias); ok && port != 0 {
		return true
	}
	if r.Container.IsHostNetworkMode {
		return preferredPort(r.Container.PublicPortMapping) != 0
	}
	return preferredPort(r.Container.PrivatePortMapping) != 0
}
