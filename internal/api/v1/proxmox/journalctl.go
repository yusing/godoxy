package proxmoxapi

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/proxmox"
	"github.com/yusing/goutils/apitypes"
	"github.com/yusing/goutils/http/websocket"
)

// e.g. ws://localhost:8889/api/v1/proxmox/journalctl?node=pve&vmid=127&service=pveproxy&service=pvedaemon&limit=10
// e.g. ws://localhost:8889/api/v1/proxmox/journalctl/pve/127?service=pveproxy&service=pvedaemon&limit=10

type JournalctlRequest struct {
	Node     string   `form:"node" uri:"node" binding:"required"`                       // Node name
	VMID     *int     `form:"vmid" uri:"vmid"`                                          // Container VMID (optional - if not provided, streams node journalctl)
	Services []string `form:"service" uri:"service"`                                    // Service names
	Limit    *int     `form:"limit" uri:"limit" default:"100" binding:"min=1,max=1000"` // Limit output lines (1-1000)
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
// @Failure		500			{object}	apitypes.ErrorResponse	"Internal server error"
// @Router		/proxmox/journalctl [get]
// @Router		/proxmox/journalctl/{node} [get]
// @Router		/proxmox/journalctl/{node}/{vmid} [get]
// @Router		/proxmox/journalctl/{node}/{vmid}/{service} [get]
func Journalctl(c *gin.Context) {
	var request JournalctlRequest
	uriErr := c.ShouldBindUri(&request)
	queryErr := c.ShouldBindQuery(&request)
	if uriErr != nil && queryErr != nil { // allow both uri and query parameters to be set
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", errors.Join(uriErr, queryErr)))
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
		reader, err = node.NodeJournalctl(c.Request.Context(), request.Services, *request.Limit)
	} else {
		reader, err = node.LXCJournalctl(c.Request.Context(), *request.VMID, request.Services, *request.Limit)
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
