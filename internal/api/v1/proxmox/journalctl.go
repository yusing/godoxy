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
	Node    string `uri:"node" binding:"required"`                        // Node name
	VMID    *int   `uri:"vmid"`                                           // Container VMID (optional - if not provided, streams node journalctl)
	Service string `uri:"service"`                                        // Service name (e.g., 'pveproxy' for node, 'container@.service' format for LXC)
	Limit   int    `query:"limit" default:"100" binding:"min=1,max=1000"` // Limit output lines (1-1000)
} //	@name	ProxmoxJournalctlRequest

// @x-id				"journalctl"
// @BasePath		/api/v1
// @Summary		Get journalctl output
// @Description	Get journalctl output for node or LXC container. If vmid is not provided, streams node journalctl.
// @Tags			proxmox,websocket
// @Accept		json
// @Produce		application/json
// @Param			node		path		string	true	"Node name"
// @Param			vmid		path		int		  false	"Container VMID (optional - if not provided, streams node journalctl)"
// @Param			service	path		string	false	"Service name (e.g., 'pveproxy' for node, 'container@.service' format for LXC)"
// @Param			limit		query		int		  false	"Limit output lines (1-1000)"
// @Success		200			string	plain	  "Journalctl output"
// @Failure		400			{object}	apitypes.ErrorResponse	"Invalid request"
// @Failure		403			{object}	apitypes.ErrorResponse	"Unauthorized"
// @Failure		404			{object}	apitypes.ErrorResponse	"Node not found"
// @Failure		500			{object}	apitypes.ErrorResponse	"Internal server error"
// @Router		/proxmox/journalctl/{node} [get]
// @Router		/proxmox/journalctl/{node}/{vmid} [get]
// @Router		/proxmox/journalctl/{node}/{vmid}/{service} [get]
func Journalctl(c *gin.Context) {
	var request JournalctlRequest
	if err := c.ShouldBindUri(&request); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}
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
		reader, err = node.NodeJournalctl(c.Request.Context(), request.Service, request.Limit)
	} else {
		reader, err = node.LXCJournalctl(c.Request.Context(), *request.VMID, request.Service, request.Limit)
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
