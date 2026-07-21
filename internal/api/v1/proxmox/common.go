package proxmoxapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/proxmox"
	"github.com/yusing/goutils/apitypes"
)

type ActionRequest struct {
	Node string `uri:"node" binding:"required"`
	VMID uint64 `uri:"vmid" binding:"required"`
} //	@name	ProxmoxVMActionRequest

func nodeFromRequest(c *gin.Context, name string) (*proxmox.Node, bool) {
	node, err := proxmox.NodeFromCtx(c.Request.Context(), name)
	if err == nil {
		return node, true
	}

	switch {
	case errors.Is(err, proxmox.ErrNodeNotFound):
		c.JSON(http.StatusNotFound, apitypes.Error("node not found", err))
	case errors.Is(err, proxmox.ErrNodeAmbiguous):
		c.JSON(http.StatusConflict, apitypes.Error("node name is ambiguous", err))
	default:
		c.Error(apitypes.InternalServerError(err, "failed to get proxmox node"))
	}
	return nil, false
}
