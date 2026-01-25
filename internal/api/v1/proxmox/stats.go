package proxmoxapi

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/proxmox"
	"github.com/yusing/goutils/apitypes"
	"github.com/yusing/goutils/http/httpheaders"
	"github.com/yusing/goutils/http/websocket"
)

type StatsRequest struct {
	Node string `uri:"node" binding:"required"`
	VMID int    `uri:"vmid" binding:"required"`
}

// @x-id				"stats"
// @BasePath		/api/v1
// @Summary		Get proxmox stats
// @Description	Get proxmox stats in format of "STATUS|CPU%%|MEM USAGE/LIMIT|MEM%%|NET I/O|BLOCK I/O"
// @Tags			proxmox,websocket
// @Accept			json
// @Produce		application/json
// @Param			path		path		StatsRequest	true	"Request"
// @Success		200			string		plain	"Stats output"
// @Failure		400			{object}	apitypes.ErrorResponse	"Invalid request"
// @Failure		403			{object}	apitypes.ErrorResponse	"Unauthorized"
// @Failure		404			{object}	apitypes.ErrorResponse	"Node not found"
// @Failure		500			{object}	apitypes.ErrorResponse	"Internal server error"
// @Router		/proxmox/stats/{node}/{vmid} [get]
func Stats(c *gin.Context) {
	var request StatsRequest
	if err := c.ShouldBindUri(&request); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}

	node, ok := proxmox.Nodes.Get(request.Node)
	if !ok {
		c.JSON(http.StatusNotFound, apitypes.Error("node not found"))
		return
	}

	isWs := httpheaders.IsWebsocket(c.Request.Header)

	reader, err := node.LXCStats(c.Request.Context(), request.VMID, isWs)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to get stats"))
		return
	}
	defer reader.Close()

	if !isWs {
		var line [128]byte
		n, err := reader.Read(line[:])
		if err != nil {
			c.Error(apitypes.InternalServerError(err, "failed to copy stats"))
			return
		}
		c.Data(http.StatusOK, "text/plain; charset=utf-8", line[:n])
		return
	}

	manager, err := websocket.NewManagerWithUpgrade(c)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to upgrade to websocket"))
		return
	}
	defer manager.Close()

	writer := manager.NewWriter(websocket.TextMessage)
	_, err = io.Copy(writer, reader)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to copy stats"))
		return
	}
}
