package proxmoxapi

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/proxmox"
	"github.com/yusing/goutils/apitypes"
	"github.com/yusing/goutils/http/websocket"
)

// e.g. ws://localhost:8889/api/v1/proxmox/tail?node=pve&vmid=127&file=/var/log/immich/web.log&file=/var/log/immich/ml.log&limit=10

type TailRequest struct {
	Node  string   `form:"node" binding:"required"`                      // Node name
	VMID  *int     `form:"vmid"`                                         // Container VMID (optional - if not provided, streams node journalctl)
	Files []string `form:"file" binding:"required,dive,filepath"`        // File paths
	Limit int      `form:"limit" default:"100" binding:"min=1,max=1000"` // Limit output lines (1-1000)
} //	@name	ProxmoxTailRequest

// @x-id				"tail"
// @BasePath		/api/v1
// @Summary		Get tail output
// @Description	Get tail output for node or LXC container. If vmid is not provided, streams node tail.
// @Tags			proxmox,websocket
// @Accept		json
// @Produce		application/json
// @Param			query		query		TailRequest	true	"Request"
// @Success		200			string	plain	  "Tail output"
// @Failure		400			{object}	apitypes.ErrorResponse	"Invalid request"
// @Failure		403			{object}	apitypes.ErrorResponse	"Unauthorized"
// @Failure		404			{object}	apitypes.ErrorResponse	"Node not found"
// @Failure		500			{object}	apitypes.ErrorResponse	"Internal server error"
// @Router		/proxmox/tail [get]
func Tail(c *gin.Context) {
	var request TailRequest
	if err := c.ShouldBindQuery(&request); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}

	node, ok := proxmox.Nodes.Get(request.Node)
	if !ok {
		c.JSON(http.StatusNotFound, apitypes.Error("node not found"))
		return
	}

	c.Status(http.StatusContinue)

	var reader io.ReadCloser
	var err error
	if request.VMID == nil {
		reader, err = node.NodeTail(c.Request.Context(), request.Files, request.Limit)
	} else {
		reader, err = node.LXCTail(c.Request.Context(), *request.VMID, request.Files, request.Limit)
	}
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to get journalctl output"))
		return
	}
	defer reader.Close()

	manager, err := websocket.NewManagerWithUpgrade(c)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to upgrade to websocket"))
		return
	}
	defer manager.Close()

	writer := manager.NewWriter(websocket.TextMessage)
	_, err = io.Copy(writer, reader)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to copy journalctl output"))
		return
	}
}
