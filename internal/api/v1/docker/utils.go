package dockerapi

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/agent/pkg/agent"
	apitypes "github.com/yusing/godoxy/internal/api/types"
	config "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/gperr"
	"github.com/yusing/godoxy/internal/net/gphttp/websocket"
	"github.com/yusing/goutils/http/httpheaders"
)

type (
	DockerClients     map[string]*docker.SharedClient
	ResultType[T any] interface {
		map[string]T | []T
	}
)

// getDockerClients returns a map of docker clients for the current config.
//
// Returns a map of docker clients by server name and an error if any.
//
// Even if there are errors, the map of docker clients might not be empty.
func getDockerClients() (DockerClients, gperr.Error) {
	cfg := config.GetInstance()

	dockerHosts := cfg.Value().Providers.Docker
	dockerClients := make(DockerClients)

	connErrs := gperr.NewBuilder("failed to connect to docker")

	for name, host := range dockerHosts {
		dockerClient, err := docker.NewClient(host)
		if err != nil {
			connErrs.Add(err)
			continue
		}
		dockerClients[name] = dockerClient
	}

	for _, agent := range agent.ListAgents() {
		dockerClient, err := docker.NewClient(agent.FakeDockerHost())
		if err != nil {
			connErrs.Add(err)
			continue
		}
		dockerClients[agent.Name] = dockerClient
	}

	return dockerClients, connErrs.Error()
}

func getDockerClient(server string) (*docker.SharedClient, bool, error) {
	cfg := config.GetInstance()
	var host string
	for name, h := range cfg.Value().Providers.Docker {
		if name == server {
			host = h
			break
		}
	}
	if host == "" {
		for _, agent := range agent.ListAgents() {
			if agent.Name == server {
				host = agent.FakeDockerHost()
				break
			}
		}
	}
	if host == "" {
		return nil, false, nil
	}
	dockerClient, err := docker.NewClient(host)
	if err != nil {
		return nil, false, err
	}
	return dockerClient, true, nil
}

// closeAllClients closes all docker clients after a delay.
//
// This is used to ensure that all docker clients are closed after the http handler returns.
func closeAllClients(dockerClients DockerClients) {
	for _, dockerClient := range dockerClients {
		dockerClient.Close()
	}
}

func handleResult[V any, T ResultType[V]](c *gin.Context, errs error, result T) {
	if errs != nil {
		if len(result) == 0 {
			c.Error(apitypes.InternalServerError(errs, "docker errors"))
			return
		}
	}
	c.JSON(http.StatusOK, result)
}

func serveHTTP[V any, T ResultType[V]](c *gin.Context, getResult func(ctx context.Context, dockerClients DockerClients) (T, gperr.Error)) {
	dockerClients, err := getDockerClients()
	if err != nil {
		handleResult[V, T](c, err, nil)
		return
	}
	defer closeAllClients(dockerClients)

	if httpheaders.IsWebsocket(c.Request.Header) {
		websocket.PeriodicWrite(c, 5*time.Second, func() (any, error) {
			return getResult(c.Request.Context(), dockerClients)
		})
	} else {
		result, err := getResult(c.Request.Context(), dockerClients)
		handleResult[V](c, err, result)
	}
}
