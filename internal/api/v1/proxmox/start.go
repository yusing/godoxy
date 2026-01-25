package proxmoxapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/proxmox"
	apitypes "github.com/yusing/goutils/apitypes"
)

// @x-id				"lxcStart"
// @BasePath		/api/v1
// @Summary		Start LXC container
// @Description	Start LXC container by node and vmid
// @Tags			proxmox
// @Produce		json
// @Param			path		path		ActionRequest	true	"Request"
// @Success		200	{object}  apitypes.SuccessResponse
// @Failure		400	{object}	apitypes.ErrorResponse "Invalid request"
// @Failure		404	{object}	apitypes.ErrorResponse "Node not found"
// @Failure		500	{object}	apitypes.ErrorResponse
// @Router		/api/v1/proxmox/lxc/:node/:vmid/start [post]
func Start(c *gin.Context) {
	var req ActionRequest
	if err := c.ShouldBindUri(&req); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}

	node, ok := proxmox.Nodes.Get(req.Node)
	if !ok {
		c.JSON(http.StatusNotFound, apitypes.Error("node not found"))
		return
	}

	if err := node.LXCAction(c.Request.Context(), req.VMID, proxmox.LXCStart); err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to start container"))
		return
	}

	c.JSON(http.StatusOK, apitypes.Success("container started"))
}
