package proxmoxapi

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/proxmox"
	"github.com/yusing/goutils/apitypes"
	"github.com/yusing/goutils/http/websocket"
)

type JournalctlRequest struct {
	Node    string `uri:"node" binding:"required"`
	VMID    int    `uri:"vmid" binding:"required"`
	Service string `uri:"service"`
	Limit   int    `query:"limit" binding:"omitempty,min=1,max=1000"`
}

// @x-id				"journalctl"
// @BasePath		/api/v1
// @Summary		Get journalctl output
// @Description	Get journalctl output
// @Tags			proxmox,websocket
// @Accept			json
// @Produce		application/json
// @Param			path		path		JournalctlRequest	true	"Request"
// @Param			limit		query  	int	false	"limit"
// @Success		200			string		plain	"Journalctl output"
// @Failure		400			{object}	apitypes.ErrorResponse	"Invalid request"
// @Failure		403			{object}	apitypes.ErrorResponse	"Unauthorized"
// @Failure		404			{object}	apitypes.ErrorResponse	"Node not found"
// @Failure		500			{object}	apitypes.ErrorResponse	"Internal server error"
// @Router		/api/v1/proxmox/journalctl/{node}/{vmid} [get]
// @Router		/api/v1/proxmox/journalctl/{node}/{vmid}/{service} [get]
func Journalctl(c *gin.Context) {
	var request JournalctlRequest
	if err := c.ShouldBindUri(&request); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}

	node, ok := proxmox.Nodes.Get(request.Node)
	if !ok {
		c.JSON(http.StatusNotFound, apitypes.Error("node not found"))
		return
	}

	manager, err := websocket.NewManagerWithUpgrade(c)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to upgrade to websocket"))
		return
	}
	defer manager.Close()

	reader, err := node.LXCJournalctl(c.Request.Context(), request.VMID, request.Service, request.Limit)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to get journalctl output"))
		return
	}
	defer reader.Close()

	writer := manager.NewWriter(websocket.TextMessage)
	_, err = io.Copy(writer, reader)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to copy journalctl output"))
		return
	}
}
