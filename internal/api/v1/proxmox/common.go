package proxmoxapi

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/proxmox"
	"github.com/yusing/goutils/apitypes"
	"github.com/yusing/goutils/http/websocket"
)

type ActionRequest struct {
	Node string `uri:"node" binding:"required"`
	VMID uint64 `uri:"vmid" binding:"required"`
} //	@name	ProxmoxVMActionRequest

func streamProxmoxWebSocket(
	c *gin.Context,
	open func(context.Context) (io.ReadCloser, error),
	openError string,
	copyError string,
) {
	streamCtx, streamCancel := context.WithCancel(c.Request.Context())
	defer streamCancel()

	reader, err := open(streamCtx)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, openError))
		return
	}
	defer reader.Close()

	manager, err := websocket.NewManagerWithUpgrade(c)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to upgrade to websocket"))
		return
	}
	defer manager.Close()
	stopStreamCancel := context.AfterFunc(manager.Context(), streamCancel)
	defer stopStreamCancel()

	writer := manager.NewWriter(websocket.TextMessage)
	if _, err := io.Copy(writer, reader); err != nil && manager.Context().Err() == nil {
		c.Error(apitypes.InternalServerError(err, copyError))
	}
}

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
