package dockerapi

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/docker"
	apitypes "github.com/yusing/goutils/apitypes"
	"github.com/yusing/goutils/http/httpheaders"
	"github.com/yusing/goutils/http/websocket"
)

type (
	DockerClients     map[string]*docker.SharedClient
	ResultType[T any] interface {
		map[string]T | []T
	}
)

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

func serveHTTP[V any, T ResultType[V]](c *gin.Context, getResult func(ctx context.Context, dockerClients DockerClients) (T, error)) {
	dockerClients := docker.Clients()
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
