package dockerapi

import (
	"net/http"

	"github.com/docker/docker/api/types/container"
	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/docker"
	apitypes "github.com/yusing/goutils/apitypes"
)

type StopRequest struct {
	ID string `json:"id" binding:"required"`
	container.StopOptions
}

// @x-id				"stop"
// @BasePath		/api/v1
// @Summary		Stop container
// @Description	Stop container by container id
// @Tags			docker
// @Produce		json
// @Param			request	body		StopRequest	true	"Request"
// @Success		200	{object}  apitypes.SuccessResponse
// @Failure		400	{object}	apitypes.ErrorResponse "Invalid request"
// @Failure		403	{object}	apitypes.ErrorResponse
// @Failure		404	{object}	apitypes.ErrorResponse "Container not found"
// @Failure		500	{object}	apitypes.ErrorResponse
// @Router			/docker/stop [post]
func Stop(c *gin.Context) {
	var req StopRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}

	dockerCfg, ok := docker.GetDockerCfgByContainerID(req.ID)
	if !ok {
		c.JSON(http.StatusNotFound, apitypes.Error("container not found"))
		return
	}

	client, err := docker.NewClient(dockerCfg)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to create docker client"))
		return
	}

	defer client.Close()

	err = client.ContainerStop(c.Request.Context(), req.ID, req.StopOptions)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to stop container"))
		return
	}

	c.JSON(http.StatusOK, apitypes.Success("container stopped"))
}
