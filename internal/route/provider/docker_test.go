package provider

import (
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/yusing/go-proxy/internal/common"
	D "github.com/yusing/go-proxy/internal/docker"
	"github.com/yusing/go-proxy/internal/route"
	T "github.com/yusing/go-proxy/internal/route/types"
	. "github.com/yusing/go-proxy/internal/utils/testing"
)

var dummyNames = []string{"/a"}

const (
	testIP       = "192.168.2.100"
	testDockerIP = "172.17.0.123"
)

func makeRoutes(cont *types.Container, dockerHostIP ...string) route.Routes {
	var p DockerProvider
	var host string
	if len(dockerHostIP) > 0 {
		host = "tcp://" + dockerHostIP[0] + ":2375"
	} else {
		host = client.DefaultDockerHost
	}
	cont.ID = "test"
	p.name = "test"
	routes := Must(p.routesFromContainerLabels(D.FromDocker(cont, host)))
	for _, r := range routes {
		r.Finalize()
	}
	return routes
}

func TestExplicitOnly(t *testing.T) {
	p := NewDockerProvider("a!", "")
	ExpectTrue(t, p.IsExplicitOnly())
}

func TestApplyLabel(t *testing.T) {
	pathPatterns := `
- /
- POST /upload/{$}
- GET /static
`[1:]
	pathPatternsExpect := []string{
		"/",
		"POST /upload/{$}",
		"GET /static",
	}
	middlewaresExpect := map[string]map[string]any{
		"request": {
			"set_headers": map[string]any{
				"X-Header": "value1",
			},
			"add_headers": map[string]any{
				"X-Header2": "value2",
			},
		},
	}
	entries := makeRoutes(&types.Container{
		Names: dummyNames,
		Labels: map[string]string{
			D.LabelAliases:          "a,b",
			D.LabelIdleTimeout:      "",
			D.LabelStopMethod:       common.StopMethodDefault,
			D.LabelStopSignal:       "SIGTERM",
			D.LabelStopTimeout:      common.StopTimeoutDefault,
			D.LabelWakeTimeout:      common.WakeTimeoutDefault,
			"proxy.*.no_tls_verify": "true",
			"proxy.*.scheme":        "https",
			"proxy.*.host":          "app",
			"proxy.*.port":          "4567",
			"proxy.a.path_patterns": pathPatterns,
			"proxy.a.middlewares.request.set_headers.X-Header":  "value1",
			"proxy.a.middlewares.request.add_headers.X-Header2": "value2",
			"proxy.a.homepage.show":                             "true",
			"proxy.a.homepage.icon":                             "png/adguard-home.png",
			"proxy.a.healthcheck.path":                          "/ping",
			"proxy.a.healthcheck.interval":                      "10s",
		},
	})

	a, ok := entries["a"]
	ExpectTrue(t, ok)
	b, ok := entries["b"]
	ExpectTrue(t, ok)

	ExpectEqual(t, a.Scheme, "https")
	ExpectEqual(t, b.Scheme, "https")

	ExpectEqual(t, a.Host, "app")
	ExpectEqual(t, b.Host, "app")

	ExpectEqual(t, a.Port.Proxy, 4567)
	ExpectEqual(t, b.Port.Proxy, 4567)

	ExpectTrue(t, a.NoTLSVerify)
	ExpectTrue(t, b.NoTLSVerify)

	ExpectDeepEqual(t, a.PathPatterns, pathPatternsExpect)
	ExpectEqual(t, len(b.PathPatterns), 0)

	ExpectDeepEqual(t, a.Middlewares, middlewaresExpect)
	ExpectEqual(t, len(b.Middlewares), 0)

	ExpectEqual(t, a.Container.IdleTimeout, "")
	ExpectEqual(t, b.Container.IdleTimeout, "")

	ExpectEqual(t, a.Container.StopTimeout, common.StopTimeoutDefault)
	ExpectEqual(t, b.Container.StopTimeout, common.StopTimeoutDefault)

	ExpectEqual(t, a.Container.StopMethod, common.StopMethodDefault)
	ExpectEqual(t, b.Container.StopMethod, common.StopMethodDefault)

	ExpectEqual(t, a.Container.WakeTimeout, common.WakeTimeoutDefault)
	ExpectEqual(t, b.Container.WakeTimeout, common.WakeTimeoutDefault)

	ExpectEqual(t, a.Container.StopSignal, "SIGTERM")
	ExpectEqual(t, b.Container.StopSignal, "SIGTERM")

	ExpectEqual(t, a.Homepage.Show, true)
	ExpectEqual(t, a.Homepage.Icon.Value, "png/adguard-home.png")
	ExpectEqual(t, a.Homepage.Icon.Extra.FileType, "png")
	ExpectEqual(t, a.Homepage.Icon.Extra.Name, "adguard-home")

	ExpectEqual(t, a.HealthCheck.Path, "/ping")
	ExpectEqual(t, a.HealthCheck.Interval, 10*time.Second)
}

func TestApplyLabelWithAlias(t *testing.T) {
	entries := makeRoutes(&types.Container{
		Names: dummyNames,
		State: "running",
		Labels: map[string]string{
			D.LabelAliases:          "a,b,c",
			"proxy.a.no_tls_verify": "true",
			"proxy.a.port":          "3333",
			"proxy.b.port":          "1234",
			"proxy.c.scheme":        "https",
		},
	})
	a, ok := entries["a"]
	ExpectTrue(t, ok)
	b, ok := entries["b"]
	ExpectTrue(t, ok)
	c, ok := entries["c"]
	ExpectTrue(t, ok)

	ExpectEqual(t, a.Scheme, "http")
	ExpectEqual(t, a.Port.Proxy, 3333)
	ExpectEqual(t, a.NoTLSVerify, true)
	ExpectEqual(t, b.Scheme, "http")
	ExpectEqual(t, b.Port.Proxy, 1234)
	ExpectEqual(t, c.Scheme, "https")
}

func TestApplyLabelWithRef(t *testing.T) {
	entries := makeRoutes(&types.Container{
		Names: dummyNames,
		State: "running",
		Labels: map[string]string{
			D.LabelAliases:    "a,b,c",
			"proxy.#1.host":   "localhost",
			"proxy.#1.port":   "4444",
			"proxy.#2.port":   "9999",
			"proxy.#3.port":   "1111",
			"proxy.#3.scheme": "https",
		},
	})
	a, ok := entries["a"]
	ExpectTrue(t, ok)
	b, ok := entries["b"]
	ExpectTrue(t, ok)
	c, ok := entries["c"]
	ExpectTrue(t, ok)

	ExpectEqual(t, a.Scheme, "http")
	ExpectEqual(t, a.Host, "localhost")
	ExpectEqual(t, a.Port.Proxy, 4444)
	ExpectEqual(t, b.Port.Proxy, 9999)
	ExpectEqual(t, c.Scheme, "https")
	ExpectEqual(t, c.Port.Proxy, 1111)
}

func TestApplyLabelWithRefIndexError(t *testing.T) {
	c := D.FromDocker(&types.Container{
		Names: dummyNames,
		State: "running",
		Labels: map[string]string{
			D.LabelAliases:    "a,b",
			"proxy.#1.host":   "localhost",
			"proxy.*.port":    "4444",
			"proxy.#4.scheme": "https",
		},
	}, "")
	var p DockerProvider
	_, err := p.routesFromContainerLabels(c)
	ExpectError(t, ErrAliasRefIndexOutOfRange, err)

	c = D.FromDocker(&types.Container{
		Names: dummyNames,
		State: "running",
		Labels: map[string]string{
			D.LabelAliases:  "a,b",
			"proxy.#0.host": "localhost",
		},
	}, "")
	_, err = p.routesFromContainerLabels(c)
	ExpectError(t, ErrAliasRefIndexOutOfRange, err)
}

func TestDynamicAliases(t *testing.T) {
	c := &types.Container{
		Names: []string{"app1"},
		State: "running",
		Labels: map[string]string{
			"proxy.app1.port":         "1234",
			"proxy.app1_backend.port": "5678",
		},
	}

	entries := makeRoutes(c)

	r, ok := entries["app1"]
	ExpectTrue(t, ok)
	ExpectEqual(t, r.Scheme, "http")
	ExpectEqual(t, r.Port.Proxy, 1234)

	r, ok = entries["app1_backend"]
	ExpectTrue(t, ok)
	ExpectEqual(t, r.Scheme, "http")
	ExpectEqual(t, r.Port.Proxy, 5678)
}

func TestDisableHealthCheck(t *testing.T) {
	c := &types.Container{
		Names: dummyNames,
		State: "running",
		Labels: map[string]string{
			"proxy.a.healthcheck.disable": "true",
			"proxy.a.port":                "1234",
		},
	}
	r, ok := makeRoutes(c)["a"]
	ExpectTrue(t, ok)
	ExpectFalse(t, r.UseHealthCheck())
}

func TestPublicIPLocalhost(t *testing.T) {
	c := &types.Container{Names: dummyNames, State: "running"}
	r, ok := makeRoutes(c)["a"]
	ExpectTrue(t, ok)
	ExpectEqual(t, r.Container.PublicHostname, "127.0.0.1")
	ExpectEqual(t, r.Host, r.Container.PublicHostname)
}

func TestPublicIPRemote(t *testing.T) {
	c := &types.Container{Names: dummyNames, State: "running"}
	raw, ok := makeRoutes(c, testIP)["a"]
	ExpectTrue(t, ok)
	ExpectEqual(t, raw.Container.PublicHostname, testIP)
	ExpectEqual(t, raw.Host, raw.Container.PublicHostname)
}

func TestPrivateIPLocalhost(t *testing.T) {
	c := &types.Container{
		Names: dummyNames,
		NetworkSettings: &types.SummaryNetworkSettings{
			Networks: map[string]*network.EndpointSettings{
				"network": {
					IPAddress: testDockerIP,
				},
			},
		},
	}
	r, ok := makeRoutes(c)["a"]
	ExpectTrue(t, ok)
	ExpectEqual(t, r.Container.PrivateHostname, testDockerIP)
	ExpectEqual(t, r.Host, r.Container.PrivateHostname)
}

func TestPrivateIPRemote(t *testing.T) {
	c := &types.Container{
		Names: dummyNames,
		State: "running",
		NetworkSettings: &types.SummaryNetworkSettings{
			Networks: map[string]*network.EndpointSettings{
				"network": {
					IPAddress: testDockerIP,
				},
			},
		},
	}
	r, ok := makeRoutes(c, testIP)["a"]
	ExpectTrue(t, ok)
	ExpectEqual(t, r.Container.PrivateHostname, "")
	ExpectEqual(t, r.Container.PublicHostname, testIP)
	ExpectEqual(t, r.Host, r.Container.PublicHostname)
}

func TestStreamDefaultValues(t *testing.T) {
	privPort := uint16(1234)
	pubPort := uint16(4567)
	privIP := "172.17.0.123"
	cont := &types.Container{
		Names: []string{"a"},
		State: "running",
		NetworkSettings: &types.SummaryNetworkSettings{
			Networks: map[string]*network.EndpointSettings{
				"network": {
					IPAddress: privIP,
				},
			},
		},
		Ports: []types.Port{
			{Type: "udp", PrivatePort: privPort, PublicPort: pubPort},
		},
	}

	t.Run("local", func(t *testing.T) {
		r, ok := makeRoutes(cont)["a"]
		ExpectTrue(t, ok)
		ExpectNoError(t, r.Validate())
		ExpectEqual(t, r.Scheme, T.Scheme("udp"))
		ExpectEqual(t, r.TargetURL().Hostname(), privIP)
		ExpectEqual(t, r.Port.Listening, 0)
		ExpectEqual(t, r.Port.Proxy, int(privPort))
	})

	t.Run("remote", func(t *testing.T) {
		r, ok := makeRoutes(cont, testIP)["a"]
		ExpectTrue(t, ok)
		ExpectNoError(t, r.Validate())
		ExpectEqual(t, r.Scheme, T.Scheme("udp"))
		ExpectEqual(t, r.TargetURL().Hostname(), testIP)
		ExpectEqual(t, r.Port.Listening, 0)
		ExpectEqual(t, r.Port.Proxy, int(pubPort))
	})
}

func TestExplicitExclude(t *testing.T) {
	r, ok := makeRoutes(&types.Container{
		Names: dummyNames,
		Labels: map[string]string{
			D.LabelAliases:          "a",
			D.LabelExclude:          "true",
			"proxy.a.no_tls_verify": "true",
		},
	}, "")["a"]
	ExpectTrue(t, ok)
	ExpectTrue(t, r.ShouldExclude())
}

func TestImplicitExcludeDatabase(t *testing.T) {
	t.Run("mount path detection", func(t *testing.T) {
		r, ok := makeRoutes(&types.Container{
			Names: dummyNames,
			Mounts: []types.MountPoint{
				{Source: "/data", Destination: "/var/lib/postgresql/data"},
			},
		})["a"]
		ExpectTrue(t, ok)
		ExpectTrue(t, r.ShouldExclude())
	})
	t.Run("exposed port detection", func(t *testing.T) {
		r, ok := makeRoutes(&types.Container{
			Names: dummyNames,
			Ports: []types.Port{
				{Type: "tcp", PrivatePort: 5432, PublicPort: 5432},
			},
		})["a"]
		ExpectTrue(t, ok)
		ExpectTrue(t, r.ShouldExclude())
	})
}
