package route

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/common"
	route "github.com/yusing/godoxy/internal/route/types"
	"github.com/yusing/godoxy/internal/types"
)

func TestRouteValidate(t *testing.T) {
	t.Run("ReservedPort", func(t *testing.T) {
		r := &Route{
			Alias:  "test",
			Scheme: route.SchemeHTTP,
			Host:   "localhost",
			Port:   route.Port{Proxy: common.ProxyHTTPPort},
		}
		err := r.Validate()
		require.Error(t, err, "Validate should return error for localhost with reserved port")
		require.ErrorContains(t, err, "reserved for godoxy")
	})

	t.Run("DisabledHealthCheckWithLoadBalancer", func(t *testing.T) {
		r := &Route{
			Alias:  "test",
			Scheme: route.SchemeHTTP,
			Host:   "example.com",
			Port:   route.Port{Proxy: 80},
			HealthCheck: types.HealthCheckConfig{
				Disable: true,
			},
			LoadBalance: &types.LoadBalancerConfig{
				Link: "test-link",
			}, // Minimal LoadBalance config with non-empty Link will be checked by UseLoadBalance
		}
		err := r.Validate()
		require.Error(t, err, "Validate should return error for disabled healthcheck with loadbalancer")
		require.ErrorContains(t, err, "cannot disable healthcheck")
	})

	t.Run("FileServerScheme", func(t *testing.T) {
		r := &Route{
			Alias:  "test",
			Scheme: route.SchemeFileServer,
			Host:   "example.com",
			Port:   route.Port{Proxy: 80},
			Root:   "/tmp", // Root is required for file server
		}
		err := r.Validate()
		require.NoError(t, err, "Validate should not return error for valid file server route")
		require.NotNil(t, r.impl, "Impl should be initialized")
	})

	t.Run("HTTPScheme", func(t *testing.T) {
		r := &Route{
			Alias:  "test",
			Scheme: route.SchemeHTTP,
			Host:   "example.com",
			Port:   route.Port{Proxy: 80},
		}
		err := r.Validate()
		require.NoError(t, err, "Validate should not return error for valid HTTP route")
		require.NotNil(t, r.impl, "Impl should be initialized")
	})

	t.Run("TCPScheme", func(t *testing.T) {
		r := &Route{
			Alias:  "test",
			Scheme: route.SchemeTCP,
			Host:   "example.com",
			Port:   route.Port{Proxy: 80, Listening: 8080},
		}
		err := r.Validate()
		require.NoError(t, err, "Validate should not return error for valid TCP route")
		require.NotNil(t, r.impl, "Impl should be initialized")
	})

	t.Run("DockerContainer", func(t *testing.T) {
		r := &Route{
			Alias:  "test",
			Scheme: route.SchemeHTTP,
			Host:   "example.com",
			Port:   route.Port{Proxy: 80},
			Metadata: Metadata{
				Container: &types.Container{
					ContainerID: "test-id",
					Image: &types.ContainerImage{
						Name: "test-image",
					},
				},
			},
		}
		err := r.Validate()
		require.NoError(t, err, "Validate should not return error for valid docker container route")
		require.NotNil(t, r.ProxyURL, "ProxyURL should be set")
	})

	t.Run("InvalidScheme", func(t *testing.T) {
		r := &Route{
			Alias:  "test",
			Scheme: 123,
			Host:   "example.com",
			Port:   route.Port{Proxy: 80},
		}
		require.Panics(t, func() {
			_ = r.Validate()
		}, "Validate should panic for invalid scheme")
	})

	t.Run("ModifiedFields", func(t *testing.T) {
		r := &Route{
			Alias:  "test",
			Scheme: route.SchemeHTTP,
			Host:   "example.com",
			Port:   route.Port{Proxy: 80},
		}
		err := r.Validate()
		require.NoError(t, err)
		require.NotNil(t, r.ProxyURL)
		require.NotNil(t, r.HealthCheck)
	})
}

func TestPreferredPort(t *testing.T) {
	ports := types.PortMapping{
		22:   {PrivatePort: 22},
		1000: {PrivatePort: 1000},
		3000: {PrivatePort: 80},
	}

	port := preferredPort(ports)
	require.Equal(t, 3000, port)
}

func TestDockerRouteDisallowAgent(t *testing.T) {
	r := &Route{
		Alias:  "test",
		Scheme: route.SchemeHTTP,
		Host:   "example.com",
		Port:   route.Port{Proxy: 80},
		Agent:  "test-agent",
		Metadata: Metadata{
			Container: &types.Container{
				ContainerID: "test-id",
				Image: &types.ContainerImage{
					Name: "test-image",
				},
			},
		},
	}
	err := r.Validate()
	require.Error(t, err, "Validate should return error for docker route with agent")
	require.ErrorContains(t, err, "specifying agent is not allowed for docker container routes")
}

func TestRouteAgent(t *testing.T) {
	r := &Route{
		Alias:  "test",
		Scheme: route.SchemeHTTP,
		Host:   "example.com",
		Port:   route.Port{Proxy: 80},
		Agent:  "test-agent",
	}
	err := r.Validate()
	require.NoError(t, err, "Validate should not return error for valid route with agent")
	require.NotNil(t, r.GetAgent(), "GetAgent should return agent")
}

func TestRouteApplyingHealthCheckDefaults(t *testing.T) {
	hc := types.HealthCheckConfig{}
	hc.ApplyDefaults(types.HealthCheckConfig{
		Interval: 15 * time.Second,
		Timeout:  10 * time.Second,
	})

	require.Equal(t, 15*time.Second, hc.Interval)
	require.Equal(t, 10*time.Second, hc.Timeout)
}

func TestRouteBindField(t *testing.T) {
	t.Run("TCPSchemeWithCustomBind", func(t *testing.T) {
		r := &Route{
			Alias:  "test-tcp",
			Scheme: route.SchemeTCP,
			Host:   "192.168.1.100",
			Port:   route.Port{Proxy: 80, Listening: 8080},
			Bind:   "192.168.1.1",
		}
		err := r.Validate()
		require.NoError(t, err, "Validate should not return error for TCP route with custom bind")
		require.NotNil(t, r.LisURL, "LisURL should be set")
		require.Equal(t, "tcp4://192.168.1.1:8080", r.LisURL.String(), "LisURL should contain custom bind address")
	})

	t.Run("UDPSchemeWithCustomBind", func(t *testing.T) {
		r := &Route{
			Alias:  "test-udp",
			Scheme: route.SchemeUDP,
			Host:   "10.0.0.1",
			Port:   route.Port{Proxy: 53, Listening: 53},
			Bind:   "10.0.0.254",
		}
		err := r.Validate()
		require.NoError(t, err, "Validate should not return error for UDP route with custom bind")
		require.NotNil(t, r.LisURL, "LisURL should be set")
		require.Equal(t, "udp4://10.0.0.254:53", r.LisURL.String(), "LisURL should contain custom bind address")
	})

	t.Run("HTTPSchemeWithoutBind", func(t *testing.T) {
		r := &Route{
			Alias:  "test-http",
			Scheme: route.SchemeHTTPS,
			Host:   "example.com",
			Port:   route.Port{Proxy: 443},
		}
		err := r.Validate()
		require.NoError(t, err, "Validate should not return error for HTTP route with bind")
		require.NotNil(t, r.LisURL, "LisURL should be set")
		require.Equal(t, "https://:0", r.LisURL.String(), "LisURL should contain bind address")
	})

	t.Run("HTTPSchemeWithBind", func(t *testing.T) {
		r := &Route{
			Alias:  "test-http",
			Scheme: route.SchemeHTTPS,
			Host:   "example.com",
			Port:   route.Port{Proxy: 443},
			Bind:   "0.0.0.0",
		}
		err := r.Validate()
		require.NoError(t, err, "Validate should not return error for HTTP route with bind")
		require.NotNil(t, r.LisURL, "LisURL should be set")
		require.Equal(t, "https://0.0.0.0:0", r.LisURL.String(), "LisURL should contain bind address")
	})

	t.Run("HTTPSchemeWithBindAndPort", func(t *testing.T) {
		r := &Route{
			Alias:  "test-http",
			Scheme: route.SchemeHTTPS,
			Host:   "example.com",
			Port:   route.Port{Listening: 8443, Proxy: 443},
			Bind:   "0.0.0.0",
		}
		err := r.Validate()
		require.NoError(t, err, "Validate should not return error for HTTP route with bind and port")
		require.NotNil(t, r.LisURL, "LisURL should be set")
		require.Equal(t, "https://0.0.0.0:8443", r.LisURL.String(), "LisURL should contain bind address and listening port")
	})

	t.Run("TCPSchemeDefaultsToZeroBind", func(t *testing.T) {
		r := &Route{
			Alias:  "test-default-bind",
			Scheme: route.SchemeTCP,
			Host:   "example.com",
			Port:   route.Port{Proxy: 80, Listening: 8080},
			Bind:   "",
		}
		err := r.Validate()
		require.NoError(t, err, "Validate should not return error for TCP route with empty bind")
		require.Equal(t, "0.0.0.0", r.Bind, "Bind should default to 0.0.0.0 for TCP scheme")
		require.NotNil(t, r.LisURL, "LisURL should be set")
		require.Equal(t, "tcp4://0.0.0.0:8080", r.LisURL.String(), "LisURL should use default bind address")
	})

	t.Run("FileServerSchemeWithBind", func(t *testing.T) {
		r := &Route{
			Alias:  "test-fileserver",
			Scheme: route.SchemeFileServer,
			Port:   route.Port{Listening: 9000},
			Root:   "/tmp",
			Bind:   "127.0.0.1",
		}
		err := r.Validate()
		require.NoError(t, err, "Validate should not return error for fileserver route with bind")
		require.NotNil(t, r.LisURL, "LisURL should be set")
		require.Equal(t, "https://127.0.0.1:9000", r.LisURL.String(), "LisURL should contain bind address")
	})
}
