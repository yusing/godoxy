package dockerapi

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/route/routes"
	"github.com/yusing/godoxy/internal/types"
	apitypes "github.com/yusing/goutils/apitypes"
	"github.com/yusing/goutils/http/httpheaders"
	"github.com/yusing/goutils/http/websocket"
	"github.com/yusing/goutils/synk"
	"github.com/yusing/goutils/task"
)

type ContainerStatsResponse container.StatsResponse // @name ContainerStatsResponse

// @x-id				"stats"
// @BasePath		/api/v1
// @Summary		Get container stats
// @Description	Get container stats by container id
// @Tags			docker,websocket
// @Produce		json
// @Param			id	path		string	true	"Container ID or route alias"
// @Success		200	{object}	ContainerStatsResponse
// @Failure		400	{object}	apitypes.ErrorResponse "Invalid request: id is required or route is not a docker container"
// @Failure		403	{object}	apitypes.ErrorResponse
// @Failure		404	{object}	apitypes.ErrorResponse "Container not found"
// @Failure		500	{object}	apitypes.ErrorResponse
// @Router			/docker/stats/{id} [get]
func Stats(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, apitypes.Error("id is required"))
		return
	}

	dockerCfg, ok := docker.GetDockerCfgByContainerID(id)
	if !ok {
		var route types.Route
		route, ok = routes.GetIncludeExcluded(id)
		if ok {
			cont := route.ContainerInfo()
			if cont == nil {
				c.JSON(http.StatusBadRequest, apitypes.Error("route is not a docker container"))
				return
			}
			dockerCfg = cont.DockerCfg
			id = cont.ContainerID
		}
	}
	if !ok {
		c.JSON(http.StatusNotFound, apitypes.Error("container or route not found"))
		return
	}

	dockerClient, err := docker.NewClient(dockerCfg)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to create docker client"))
		return
	}
	defer dockerClient.Close()

	if httpheaders.IsWebsocket(c.Request.Header) {
		stats, err := dockerClient.ContainerStats(c.Request.Context(), id, client.ContainerStatsOptions{Stream: true})
		if err != nil {
			c.Error(apitypes.InternalServerError(err, "failed to get container stats"))
			return
		}
		defer stats.Body.Close()

		manager, err := websocket.NewManagerWithUpgrade(c)
		if err != nil {
			c.Error(apitypes.InternalServerError(err, "failed to create websocket manager"))
			return
		}
		defer manager.Close()

		buf := synk.GetSizedBytesPool().GetSized(4096)
		defer synk.GetSizedBytesPool().Put(buf)

		for {
			select {
			case <-manager.Done():
				return
			default:
				_, err = io.CopyBuffer(manager.NewWriter(websocket.TextMessage), stats.Body, buf)
				if err != nil {
					if errors.Is(err, context.Canceled) || errors.Is(err, task.ErrProgramExiting) {
						return
					}
					c.Error(apitypes.InternalServerError(err, "failed to copy container stats"))
					return
				}
			}
		}
	}

	stats, err := dockerClient.ContainerStats(c.Request.Context(), id, client.ContainerStatsOptions{Stream: false})
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to get container stats"))
		return
	}
	defer stats.Body.Close()

	_, err = io.Copy(c.Writer, stats.Body)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to copy container stats"))
		return
	}
}
