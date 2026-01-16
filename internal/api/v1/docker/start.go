package dockerapi

import (
	"net/http"

	"github.com/docker/docker/api/types/container"
	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/docker"
	apitypes "github.com/yusing/goutils/apitypes"
)

type StartRequest struct {
	ID string `json:"id" binding:"required"`
	container.StartOptions
}

// @x-id				"start"
// @BasePath		/api/v1
// @Summary		Start container
// @Description	Start container by container id
// @Tags			docker
// @Produce		json
// @Param			request	body		StartRequest	true	"Request"
// @Success		200	{object}  apitypes.SuccessResponse
// @Failure		400	{object}	apitypes.ErrorResponse "Invalid request"
// @Failure		403	{object}	apitypes.ErrorResponse
// @Failure		404	{object}	apitypes.ErrorResponse "Container not found"
// @Failure		500	{object}	apitypes.ErrorResponse
// @Router			/docker/start [post]
func Start(c *gin.Context) {
	var req StartRequest
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

	err = client.ContainerStart(c.Request.Context(), req.ID, req.StartOptions)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to start container"))
		return
	}

	c.JSON(http.StatusOK, apitypes.Success("container started"))
}
