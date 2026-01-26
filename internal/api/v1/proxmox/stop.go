package proxmoxapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/proxmox"
	apitypes "github.com/yusing/goutils/apitypes"
)

// @x-id				"lxcStop"
// @BasePath		/api/v1
// @Summary		Stop LXC container
// @Description	Stop LXC container by node and vmid
// @Tags			proxmox
// @Produce		json
// @Param			path		path		ActionRequest	true	"Request"
// @Success		200	{object}  apitypes.SuccessResponse
// @Failure		400	{object}	apitypes.ErrorResponse "Invalid request"
// @Failure		404	{object}	apitypes.ErrorResponse "Node not found"
// @Failure		500	{object}	apitypes.ErrorResponse
// @Router		/proxmox/lxc/:node/:vmid/stop [post]
func Stop(c *gin.Context) {
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

	if err := node.LXCAction(c.Request.Context(), req.VMID, proxmox.LXCShutdown); err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to stop container"))
		return
	}

	c.JSON(http.StatusOK, apitypes.Success("container stopped"))
}
