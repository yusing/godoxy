package provider

import (
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	D "github.com/yusing/go-proxy/internal/docker"
	"github.com/yusing/go-proxy/internal/route"
	T "github.com/yusing/go-proxy/internal/route/types"
	expect "github.com/yusing/go-proxy/internal/utils/testing"
)

var dummyNames = []string{"/a"}

const (
	testIP       = "192.168.2.100"
	testDockerIP = "172.17.0.123"
)

func makeRoutes(cont *container.SummaryTrimmed, dockerHostIP ...string) route.Routes {
	var p DockerProvider
	var host string
	if len(dockerHostIP) > 0 {
		host = "tcp://" + dockerHostIP[0] + ":2375"
	} else {
		host = client.DefaultDockerHost
	}
	cont.ID = "test"
	p.name = "test"
	routes := expect.Must(p.routesFromContainerLabels(D.FromDocker(cont, host)))
	for _, r := range routes {
		r.Finalize()
	}
	return routes
}

func TestExplicitOnly(t *testing.T) {
	p := NewDockerProvider("a!", "")
	expect.True(t, p.IsExplicitOnly())
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
	entries := makeRoutes(&container.SummaryTrimmed{
		Names: dummyNames,
		Labels: map[string]string{
			D.LabelAliases:          "a,b",
			D.LabelIdleTimeout:      "10s",
			D.LabelStopMethod:       "stop",
			D.LabelStopSignal:       "SIGTERM",
			D.LabelStopTimeout:      "1h",
			D.LabelWakeTimeout:      "10s",
			"proxy.*.no_tls_verify": "true",
			"proxy.*.scheme":        "https",
			"proxy.*.host":          "app",
			"proxy.*.port":          "4567",
			"proxy.a.path_patterns": pathPatterns,
			"proxy.a.middlewares.request.set_headers.X-Header":  "value1",
			"proxy.a.middlewares.request.add_headers.X-Header2": "value2",
			"proxy.a.homepage.show":                             "true",
			"proxy.a.homepage.icon":                             "@selfhst/adguard-home.png",
			"proxy.a.healthcheck.path":                          "/ping",
			"proxy.a.healthcheck.interval":                      "10s",
		},
	})

	a, ok := entries["a"]
	expect.True(t, ok)
	b, ok := entries["b"]
	expect.True(t, ok)

	expect.Equal(t, a.Scheme, "https")
	expect.Equal(t, b.Scheme, "https")

	expect.Equal(t, a.Host, "app")
	expect.Equal(t, b.Host, "app")

	expect.Equal(t, a.Port.Proxy, 4567)
	expect.Equal(t, b.Port.Proxy, 4567)

	expect.True(t, a.NoTLSVerify)
	expect.True(t, b.NoTLSVerify)

	expect.Equal(t, a.PathPatterns, pathPatternsExpect)
	expect.Equal(t, len(b.PathPatterns), 0)

	expect.Equal(t, a.Middlewares, middlewaresExpect)
	expect.Equal(t, len(b.Middlewares), 0)

	expect.NotNil(t, a.Container)
	expect.NotNil(t, b.Container)
	expect.NotNil(t, a.Container.IdlewatcherConfig)
	expect.NotNil(t, b.Container.IdlewatcherConfig)

	expect.Equal(t, a.Container.IdlewatcherConfig.IdleTimeout, 10*time.Second)
	expect.Equal(t, b.Container.IdlewatcherConfig.IdleTimeout, 10*time.Second)
	expect.Equal(t, a.Container.IdlewatcherConfig.StopTimeout, time.Hour)
	expect.Equal(t, b.Container.IdlewatcherConfig.StopTimeout, time.Hour)
	expect.Equal(t, a.Container.IdlewatcherConfig.StopMethod, "stop")
	expect.Equal(t, b.Container.IdlewatcherConfig.StopMethod, "stop")
	expect.Equal(t, a.Container.IdlewatcherConfig.WakeTimeout, 10*time.Second)
	expect.Equal(t, b.Container.IdlewatcherConfig.WakeTimeout, 10*time.Second)
	expect.Equal(t, a.Container.IdlewatcherConfig.StopSignal, "SIGTERM")
	expect.Equal(t, b.Container.IdlewatcherConfig.StopSignal, "SIGTERM")

	expect.Equal(t, a.Homepage.Show, true)
	expect.Equal(t, a.Homepage.Icon.Extra.FileType, "png")
	expect.Equal(t, a.Homepage.Icon.Extra.Ref, "adguard-home")

	expect.Equal(t, a.HealthCheck.Path, "/ping")
	expect.Equal(t, a.HealthCheck.Interval, 10*time.Second)
}

func TestApplyLabelWithAlias(t *testing.T) {
	entries := makeRoutes(&container.SummaryTrimmed{
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
	expect.True(t, ok)
	b, ok := entries["b"]
	expect.True(t, ok)
	c, ok := entries["c"]
	expect.True(t, ok)

	expect.Equal(t, a.Scheme, "http")
	expect.Equal(t, a.Port.Proxy, 3333)
	expect.Equal(t, a.NoTLSVerify, true)
	expect.Equal(t, b.Scheme, "http")
	expect.Equal(t, b.Port.Proxy, 1234)
	expect.Equal(t, c.Scheme, "https")
}

func TestApplyLabelWithRef(t *testing.T) {
	entries := makeRoutes(&container.SummaryTrimmed{
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
	expect.True(t, ok)
	b, ok := entries["b"]
	expect.True(t, ok)
	c, ok := entries["c"]
	expect.True(t, ok)

	expect.Equal(t, a.Scheme, "http")
	expect.Equal(t, a.Host, "localhost")
	expect.Equal(t, a.Port.Proxy, 4444)
	expect.Equal(t, b.Port.Proxy, 9999)
	expect.Equal(t, c.Scheme, "https")
	expect.Equal(t, c.Port.Proxy, 1111)
}

func TestApplyLabelWithRefIndexError(t *testing.T) {
	c := D.FromDocker(&container.SummaryTrimmed{
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
	expect.ErrorIs(t, ErrAliasRefIndexOutOfRange, err)

	c = D.FromDocker(&container.SummaryTrimmed{
		Names: dummyNames,
		State: "running",
		Labels: map[string]string{
			D.LabelAliases:  "a,b",
			"proxy.#0.host": "localhost",
		},
	}, "")
	_, err = p.routesFromContainerLabels(c)
	expect.ErrorIs(t, ErrAliasRefIndexOutOfRange, err)
}

func TestDynamicAliases(t *testing.T) {
	c := &container.SummaryTrimmed{
		Names: []string{"app1"},
		State: "running",
		Labels: map[string]string{
			"proxy.app1.port":         "1234",
			"proxy.app1_backend.port": "5678",
		},
	}

	entries := makeRoutes(c)

	r, ok := entries["app1"]
	expect.True(t, ok)
	expect.Equal(t, r.Scheme, "http")
	expect.Equal(t, r.Port.Proxy, 1234)

	r, ok = entries["app1_backend"]
	expect.True(t, ok)
	expect.Equal(t, r.Scheme, "http")
	expect.Equal(t, r.Port.Proxy, 5678)
}

func TestDisableHealthCheck(t *testing.T) {
	c := &container.SummaryTrimmed{
		Names: dummyNames,
		State: "running",
		Labels: map[string]string{
			"proxy.a.healthcheck.disable": "true",
			"proxy.a.port":                "1234",
		},
	}
	r, ok := makeRoutes(c)["a"]
	expect.True(t, ok)
	expect.False(t, r.UseHealthCheck())
}

func TestPublicIPLocalhost(t *testing.T) {
	c := &container.SummaryTrimmed{Names: dummyNames, State: "running"}
	r, ok := makeRoutes(c)["a"]
	expect.True(t, ok)
	expect.Equal(t, r.Container.PublicHostname, "127.0.0.1")
	expect.Equal(t, r.Host, r.Container.PublicHostname)
}

func TestPublicIPRemote(t *testing.T) {
	c := &container.SummaryTrimmed{Names: dummyNames, State: "running"}
	raw, ok := makeRoutes(c, testIP)["a"]
	expect.True(t, ok)
	expect.Equal(t, raw.Container.PublicHostname, testIP)
	expect.Equal(t, raw.Host, raw.Container.PublicHostname)
}

func TestPrivateIPLocalhost(t *testing.T) {
	c := &container.SummaryTrimmed{
		Names: dummyNames,
		NetworkSettings: &container.NetworkSettingsSummaryTrimmed{
			Networks: map[string]*struct{ IPAddress string }{
				"network": {
					IPAddress: testDockerIP,
				},
			},
		},
	}
	r, ok := makeRoutes(c)["a"]
	expect.True(t, ok)
	expect.Equal(t, r.Container.PrivateHostname, testDockerIP)
	expect.Equal(t, r.Host, r.Container.PrivateHostname)
}

func TestPrivateIPRemote(t *testing.T) {
	c := &container.SummaryTrimmed{
		Names: dummyNames,
		State: "running",
		NetworkSettings: &container.NetworkSettingsSummaryTrimmed{
			Networks: map[string]*struct{ IPAddress string }{
				"network": {
					IPAddress: testDockerIP,
				},
			},
		},
	}
	r, ok := makeRoutes(c, testIP)["a"]
	expect.True(t, ok)
	expect.Equal(t, r.Container.PrivateHostname, "")
	expect.Equal(t, r.Container.PublicHostname, testIP)
	expect.Equal(t, r.Host, r.Container.PublicHostname)
}

func TestStreamDefaultValues(t *testing.T) {
	privPort := uint16(1234)
	pubPort := uint16(4567)
	privIP := "172.17.0.123"
	cont := &container.SummaryTrimmed{
		Names: []string{"a"},
		State: "running",
		NetworkSettings: &container.NetworkSettingsSummaryTrimmed{
			Networks: map[string]*struct{ IPAddress string }{
				"network": {
					IPAddress: privIP,
				},
			},
		},
		Ports: []container.Port{
			{Type: "udp", PrivatePort: privPort, PublicPort: pubPort},
		},
	}

	t.Run("local", func(t *testing.T) {
		r, ok := makeRoutes(cont)["a"]
		expect.True(t, ok)
		expect.NoError(t, r.Validate())
		expect.Equal(t, r.Scheme, T.Scheme("udp"))
		expect.Equal(t, r.TargetURL().Hostname(), privIP)
		expect.Equal(t, r.Port.Listening, 0)
		expect.Equal(t, r.Port.Proxy, int(privPort))
	})

	t.Run("remote", func(t *testing.T) {
		r, ok := makeRoutes(cont, testIP)["a"]
		expect.True(t, ok)
		expect.NoError(t, r.Validate())
		expect.Equal(t, r.Scheme, T.Scheme("udp"))
		expect.Equal(t, r.TargetURL().Hostname(), testIP)
		expect.Equal(t, r.Port.Listening, 0)
		expect.Equal(t, r.Port.Proxy, int(pubPort))
	})
}

func TestExplicitExclude(t *testing.T) {
	r, ok := makeRoutes(&container.SummaryTrimmed{
		Names: dummyNames,
		Labels: map[string]string{
			D.LabelAliases:          "a",
			D.LabelExclude:          "true",
			"proxy.a.no_tls_verify": "true",
		},
	}, "")["a"]
	expect.True(t, ok)
	expect.True(t, r.ShouldExclude())
}

func TestImplicitExcludeDatabase(t *testing.T) {
	t.Run("mount path detection", func(t *testing.T) {
		r, ok := makeRoutes(&container.SummaryTrimmed{
			Names: dummyNames,
			Mounts: []container.MountPointTrimmed{
				{Destination: "/var/lib/postgresql/data"},
			},
		})["a"]
		expect.True(t, ok)
		expect.True(t, r.ShouldExclude())
	})
	t.Run("exposed port detection", func(t *testing.T) {
		r, ok := makeRoutes(&container.SummaryTrimmed{
			Names: dummyNames,
			Ports: []container.Port{
				{Type: "tcp", PrivatePort: 5432, PublicPort: 5432},
			},
		})["a"]
		expect.True(t, ok)
		expect.True(t, r.ShouldExclude())
	})
}
