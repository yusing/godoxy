package dockerapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yusing/go-proxy/agent/pkg/agent"
	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/docker"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/net/gphttp/gpwebsocket"
	"github.com/yusing/go-proxy/internal/net/gphttp/httpheaders"
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
		dockerClients[agent.Name()] = dockerClient
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
			if agent.Name() == server {
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

func handleResult[V any, T ResultType[V]](w http.ResponseWriter, errs error, result T) {
	if errs != nil {
		gperr.LogError("docker errors", errs)
		if len(result) == 0 {
			http.Error(w, "docker errors", http.StatusInternalServerError)
			return
		}
	}
	json.NewEncoder(w).Encode(result) //nolint
}

func serveHTTP[V any, T ResultType[V]](w http.ResponseWriter, r *http.Request, getResult func(ctx context.Context, dockerClients DockerClients) (T, gperr.Error)) {
	dockerClients, err := getDockerClients()
	if err != nil {
		handleResult[V, T](w, err, nil)
		return
	}
	defer closeAllClients(dockerClients)

	if httpheaders.IsWebsocket(r.Header) {
		gpwebsocket.Periodic(w, r, 5*time.Second, func(conn *websocket.Conn) error {
			result, err := getResult(r.Context(), dockerClients)
			if err != nil {
				return err
			}
			return conn.WriteJSON(result)
		})
	} else {
		result, err := getResult(r.Context(), dockerClients)
		handleResult[V](w, err, result)
	}
}
