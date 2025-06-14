package route

import (
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/docker"
	loadbalance "github.com/yusing/go-proxy/internal/net/gphttp/loadbalancer/types"
	route "github.com/yusing/go-proxy/internal/route/types"
	expect "github.com/yusing/go-proxy/internal/utils/testing"
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
		expect.HasError(t, err, "Validate should return error for localhost with reserved port")
		expect.ErrorContains(t, err, "reserved for godoxy")
	})

	t.Run("ListeningPortWithHTTP", func(t *testing.T) {
		r := &Route{
			Alias:  "test",
			Scheme: route.SchemeHTTP,
			Host:   "example.com",
			Port:   route.Port{Proxy: 80, Listening: 1234},
		}
		err := r.Validate()
		expect.HasError(t, err, "Validate should return error for HTTP scheme with listening port")
		expect.ErrorContains(t, err, "unexpected listening port")
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
		expect.HasError(t, err, "Validate should return error for disabled healthcheck with loadbalancer")
		expect.ErrorContains(t, err, "cannot disable healthcheck")
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
		expect.NoError(t, err, "Validate should not return error for valid file server route")
		expect.NotNil(t, r.impl, "Impl should be initialized")
	})

	t.Run("HTTPScheme", func(t *testing.T) {
		r := &Route{
			Alias:  "test",
			Scheme: route.SchemeHTTP,
			Host:   "example.com",
			Port:   route.Port{Proxy: 80},
		}
		err := r.Validate()
		expect.NoError(t, err, "Validate should not return error for valid HTTP route")
		expect.NotNil(t, r.impl, "Impl should be initialized")
	})

	t.Run("TCPScheme", func(t *testing.T) {
		r := &Route{
			Alias:  "test",
			Scheme: route.SchemeTCP,
			Host:   "example.com",
			Port:   route.Port{Proxy: 80, Listening: 8080},
		}
		err := r.Validate()
		expect.NoError(t, err, "Validate should not return error for valid TCP route")
		expect.NotNil(t, r.impl, "Impl should be initialized")
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
		expect.NoError(t, err, "Validate should not return error for valid docker container route")
		expect.NotNil(t, r.ProxyURL, "ProxyURL should be set")
	})

	t.Run("InvalidScheme", func(t *testing.T) {
		r := &Route{
			Alias:  "test",
			Scheme: "invalid",
			Host:   "example.com",
			Port:   route.Port{Proxy: 80},
		}
		expect.Panics(t, func() {
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
		expect.NoError(t, err)
		expect.NotNil(t, r.ProxyURL)
		expect.NotNil(t, r.HealthCheck)
	})
}

func TestPreferredPort(t *testing.T) {
	ports := map[int]container.Port{
		22:   {PrivatePort: 22},
		1000: {PrivatePort: 1000},
		3000: {PrivatePort: 80},
	}

	port := preferredPort(ports)
	expect.Equal(t, port, 3000)
}

func TestDockerRouteDisallowAgent(t *testing.T) {
	r := &Route{
		Alias:  "test",
		Scheme: route.SchemeHTTP,
		Host:   "example.com",
		Port:   route.Port{Proxy: 80},
		Agent:  "test-agent",
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
	expect.HasError(t, err, "Validate should return error for docker route with agent")
	expect.ErrorContains(t, err, "specifying agent is not allowed for docker container routes")
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
	expect.NoError(t, err, "Validate should not return error for valid route with agent")
	expect.NotNil(t, r.GetAgent(), "GetAgent should return agent")
}
