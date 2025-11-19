package dockerapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/docker"
	apitypes "github.com/yusing/goutils/apitypes"
)

// @x-id				"restart"
// @BasePath		/api/v1
// @Summary		Restart container
// @Description	Restart container by container id
// @Tags			docker
// @Produce		json
// @Param			request	body		StopRequest	true	"Request"
// @Success		200	{object}  apitypes.SuccessResponse
// @Failure		400	{object}	apitypes.ErrorResponse "Invalid request"
// @Failure		403	{object}	apitypes.ErrorResponse
// @Failure		404	{object}	apitypes.ErrorResponse "Container not found"
// @Failure		500	{object}	apitypes.ErrorResponse
// @Router			/docker/restart [post]
func Restart(c *gin.Context) {
	var req StopRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}

	dockerHost, ok := docker.GetDockerHostByContainerID(req.ID)
	if !ok {
		c.JSON(http.StatusNotFound, apitypes.Error("container not found"))
		return
	}

	client, err := docker.NewClient(dockerHost)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to create docker client"))
		return
	}

	defer client.Close()

	err = client.ContainerRestart(c.Request.Context(), req.ID, req.StopOptions)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to restart container"))
		return
	}

	c.JSON(http.StatusOK, apitypes.Success("container restarted"))
}
