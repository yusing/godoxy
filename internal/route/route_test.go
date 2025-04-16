package route

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/docker"
	loadbalance "github.com/yusing/go-proxy/internal/net/gphttp/loadbalancer/types"
	route "github.com/yusing/go-proxy/internal/route/types"
	"github.com/yusing/go-proxy/internal/watcher/health"
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
		require.Contains(t, err.Error(), "reserved for godoxy")
	})

	t.Run("ListeningPortWithHTTP", func(t *testing.T) {
		r := &Route{
			Alias:  "test",
			Scheme: route.SchemeHTTP,
			Host:   "example.com",
			Port:   route.Port{Proxy: 80, Listening: 1234},
		}
		err := r.Validate()
		require.Error(t, err, "Validate should return error for HTTP scheme with listening port")
		require.Contains(t, err.Error(), "unexpected listening port")
	})

	t.Run("DisabledHealthCheckWithLoadBalancer", func(t *testing.T) {
		r := &Route{
			Alias:  "test",
			Scheme: route.SchemeHTTP,
			Host:   "example.com",
			Port:   route.Port{Proxy: 80},
			HealthCheck: &health.HealthCheckConfig{
				Disable: true,
			},
			LoadBalance: &loadbalance.Config{
				Link: "test-link",
			}, // Minimal LoadBalance config with non-empty Link will be checked by UseLoadBalance
		}
		err := r.Validate()
		require.Error(t, err, "Validate should return error for disabled healthcheck with loadbalancer")
		require.Contains(t, err.Error(), "cannot disable healthcheck")
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
				Container: &docker.Container{
					ContainerID: "test-id",
					Image: &docker.ContainerImage{
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
			Scheme: "invalid",
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
