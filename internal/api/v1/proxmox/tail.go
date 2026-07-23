package proxmoxapi

import (
	"context"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yusing/goutils/apitypes"
	"github.com/yusing/goutils/http/httpheaders"
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
// @Failure		409			{object}	apitypes.ErrorResponse	"Node name is ambiguous"
// @Failure		500			{object}	apitypes.ErrorResponse	"Internal server error"
// @Router		/proxmox/tail [get]
func Tail(c *gin.Context) {
	if !httpheaders.IsWebsocket(c.Request.Header) {
		c.JSON(http.StatusBadRequest, apitypes.Error("websocket required"))
		return
	}

	var request TailRequest
	if err := c.ShouldBindQuery(&request); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}

	node, ok := nodeFromRequest(c, request.Node)
	if !ok {
		return
	}

	c.Status(http.StatusContinue)

	streamProxmoxWebSocket(
		c,
		func(ctx context.Context) (io.ReadCloser, error) {
			if request.VMID == nil {
				return node.NodeTail(ctx, request.Files, request.Limit)
			}
			return node.LXCTail(ctx, *request.VMID, request.Files, request.Limit)
		},
		"failed to get journalctl output",
		"failed to copy journalctl output",
	)
}
