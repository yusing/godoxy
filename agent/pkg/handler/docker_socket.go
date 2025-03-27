package handler

import (
	"net/http"
	"net/url"

	"github.com/docker/docker/client"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/docker"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/net/gphttp/reverseproxy"
	"github.com/yusing/go-proxy/internal/net/types"
)

func serviceUnavailable(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "docker socket is not available", http.StatusServiceUnavailable)
}

func DockerSocketHandler() http.HandlerFunc {
	dockerClient, err := docker.NewClient(common.DockerHostFromEnv)
	if err != nil {
		logging.Warn().Err(err).Msg("failed to connect to docker client")
		return serviceUnavailable
	}
	rp := reverseproxy.NewReverseProxy("docker", types.NewURL(&url.URL{
		Scheme: "http",
		Host:   client.DummyHost,
	}), dockerClient.HTTPClient().Transport)

	return rp.ServeHTTP
}
