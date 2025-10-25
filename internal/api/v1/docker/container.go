package dockerapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/docker"
	apitypes "github.com/yusing/goutils/apitypes"
)

// @x-id				"container"
// @BasePath		/api/v1
// @Summary		Get container
// @Description	Get container by container id
// @Tags			docker
// @Produce		json
// @Param			id	path		string	true	"Container ID"
// @Success		200	{object}		Container
// @Failure		400	{object}	apitypes.ErrorResponse "ID is required"
// @Failure		403	{object}	apitypes.ErrorResponse
// @Failure		404	{object}	apitypes.ErrorResponse "Container not found"
// @Failure		500	{object}	apitypes.ErrorResponse
// @Router			/docker/container/{id} [get]
func GetContainer(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, apitypes.Error("id is required"))
		return
	}

	dockerHost, ok := docker.GetDockerHostByContainerID(id)
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

	cont, err := client.ContainerInspect(c.Request.Context(), id)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to inspect container"))
		return
	}

	var state ContainerState
	if cont.State != nil {
		state = cont.State.Status
	}

	c.JSON(http.StatusOK, &Container{
		Server: dockerHost,
		Name:   cont.Name,
		ID:     cont.ID,
		Image:  cont.Image,
		State:  state,
	})
}
