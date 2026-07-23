package proxmoxapi

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yusing/goutils/apitypes"
	"github.com/yusing/goutils/http/httpheaders"
)

// e.g. ws://localhost:8889/api/v1/proxmox/journalctl?node=pve&vmid=127&service=pveproxy&service=pvedaemon&limit=10
// e.g. ws://localhost:8889/api/v1/proxmox/journalctl/pve/127?service=pveproxy&service=pvedaemon&limit=10

type JournalctlRequest struct {
	Node     string   `form:"node" uri:"node" binding:"required"`                                 // Node name
	VMID     *int     `form:"vmid" uri:"vmid"`                                                    // Container VMID (optional - if not provided, streams node journalctl)
	Services []string `form:"service" uri:"service"`                                              // Service names
	Limit    *int     `form:"limit" uri:"limit" default:"100" binding:"omitempty,min=1,max=1000"` // Limit output lines (1-1000)
} //	@name	ProxmoxJournalctlRequest

// @x-id				"journalctl"
// @BasePath		/api/v1
// @Summary		Get journalctl output
// @Description	Get journalctl output for node or LXC container. If vmid is not provided, streams node journalctl.
// @Tags			proxmox,websocket
// @Accept		json
// @Produce		application/json
// @Param			query		query		JournalctlRequest	true	"Request"
// @Param			path		path		JournalctlRequest	true	"Request"
// @Success		200			string	plain	  "Journalctl output"
// @Failure		400			{object}	apitypes.ErrorResponse	"Invalid request"
// @Failure		403			{object}	apitypes.ErrorResponse	"Unauthorized"
// @Failure		404			{object}	apitypes.ErrorResponse	"Node not found"
// @Failure		409			{object}	apitypes.ErrorResponse	"Node name is ambiguous"
// @Failure		500			{object}	apitypes.ErrorResponse	"Internal server error"
// @Router		/proxmox/journalctl [get]
// @Router		/proxmox/journalctl/{node} [get]
// @Router		/proxmox/journalctl/{node}/{vmid} [get]
// @Router		/proxmox/journalctl/{node}/{vmid}/{service} [get]
func Journalctl(c *gin.Context) {
	if !httpheaders.IsWebsocket(c.Request.Header) {
		c.JSON(http.StatusBadRequest, apitypes.Error("websocket required"))
		return
	}

	var request JournalctlRequest
	uriErr := c.ShouldBindUri(&request)
	queryErr := c.ShouldBindQuery(&request)
	if uriErr != nil && queryErr != nil { // allow both uri and query parameters to be set
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", errors.Join(uriErr, queryErr)))
		return
	}

	if request.Limit == nil {
		request.Limit = new(100)
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
				return node.NodeJournalctl(ctx, request.Services, *request.Limit)
			}
			return node.LXCJournalctl(ctx, *request.VMID, request.Services, *request.Limit)
		},
		"failed to get journalctl output",
		"failed to copy journalctl output",
	)
}
