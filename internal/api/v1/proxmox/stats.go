package proxmoxapi

import (
	"context"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yusing/goutils/apitypes"
	"github.com/yusing/goutils/http/httpheaders"
)

type StatsRequest ActionRequest

// @x-id			"nodeStats"
// @BasePath	/api/v1
// @Summary		Get proxmox node stats
// @Description	Get proxmox node stats in json
// @Tags			proxmox,websocket
// @Produce		application/json
// @Param			node		path	  	string	true	"Node name"
// @Success		200			{object}	proxmox.NodeStats	      "Stats output"
// @Failure		400			{object}	apitypes.ErrorResponse	"Invalid request"
// @Failure		403			{object}	apitypes.ErrorResponse	"Unauthorized"
// @Failure		404			{object}	apitypes.ErrorResponse	"Node not found"
// @Failure		409			{object}	apitypes.ErrorResponse	"Node name is ambiguous"
// @Failure		500			{object}	apitypes.ErrorResponse	"Internal server error"
// @Router		/proxmox/stats/{node} [get]
func NodeStats(c *gin.Context) {
	nodeName := c.Param("node")
	if nodeName == "" {
		c.JSON(http.StatusBadRequest, apitypes.Error("node name is required"))
		return
	}

	node, ok := nodeFromRequest(c, nodeName)
	if !ok {
		return
	}

	isWs := httpheaders.IsWebsocket(c.Request.Header)
	if isWs {
		streamProxmoxWebSocket(
			c,
			func(ctx context.Context) (io.ReadCloser, error) {
				return node.NodeStats(ctx, true)
			},
			"failed to get stats",
			"failed to copy stats",
		)
		return
	}

	reader, err := node.NodeStats(c.Request.Context(), false)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to get stats"))
		return
	}
	defer reader.Close()

	var line [512]byte
	n, err := reader.Read(line[:])
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to copy stats"))
		return
	}
	c.Data(http.StatusOK, "application/json", line[:n])
}

// @x-id			"vmStats"
// @BasePath	/api/v1
// @Summary		Get proxmox VM stats
// @Description	Get proxmox VM stats in format of "STATUS|CPU%%|MEM USAGE/LIMIT|MEM%%|NET I/O|BLOCK I/O"
// @Tags			proxmox,websocket
// @Produce		text/plain
// @Param			path		path		StatsRequest	true	"Request"
// @Success		200			string		plain	"Stats output"
// @Failure		400			{object}	apitypes.ErrorResponse	"Invalid request"
// @Failure		403			{object}	apitypes.ErrorResponse	"Unauthorized"
// @Failure		404			{object}	apitypes.ErrorResponse	"Node not found"
// @Failure		409			{object}	apitypes.ErrorResponse	"Node name is ambiguous"
// @Failure		500			{object}	apitypes.ErrorResponse	"Internal server error"
// @Router		/proxmox/stats/{node}/{vmid} [get]
func VMStats(c *gin.Context) {
	var request StatsRequest
	if err := c.ShouldBindUri(&request); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}

	node, ok := nodeFromRequest(c, request.Node)
	if !ok {
		return
	}

	isWs := httpheaders.IsWebsocket(c.Request.Header)
	if isWs {
		streamProxmoxWebSocket(
			c,
			func(ctx context.Context) (io.ReadCloser, error) {
				return node.LXCStats(ctx, request.VMID, true)
			},
			"failed to get stats",
			"failed to copy stats",
		)
		return
	}

	reader, err := node.LXCStats(c.Request.Context(), request.VMID, false)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to get stats"))
		return
	}
	defer reader.Close()

	var line [128]byte
	n, err := reader.Read(line[:])
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to copy stats"))
		return
	}
	c.Data(http.StatusOK, "text/plain; charset=utf-8", line[:n])
}
