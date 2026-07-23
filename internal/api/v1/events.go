package v1

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	apitypes "github.com/yusing/goutils/apitypes"
	"github.com/yusing/goutils/events"
	"github.com/yusing/goutils/http/httpheaders"
	"github.com/yusing/goutils/http/websocket"
)

// @x-id			"events"
// @BasePath	/api/v1
// @Summary		Get events history
// @Tags			v1
// @Accept		json
// @Produce		json
// @Success		200		{array}		events.Event
// @Failure		403		{object}	apitypes.ErrorResponse "Forbidden: unauthorized"
// @Failure		500		{object}	apitypes.ErrorResponse "Internal Server Error: internal error"
// @Router			/events [get]
func Events(c *gin.Context) {
	history := events.FromCtx(c.Request.Context())
	if !httpheaders.IsWebsocket(c.Request.Header) {
		c.JSON(http.StatusOK, history.Get())
		return
	}

	manager, err := websocket.NewManagerWithUpgrade(c)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to upgrade to websocket"))
		return
	}
	defer manager.Close()

	writer := manager.NewWriter(websocket.TextMessage)
	err = history.ListenJSON(c.Request.Context(), writer)
	if err != nil && !errors.Is(err, context.Canceled) {
		c.Error(apitypes.InternalServerError(err, "failed to listen to events"))
		return
	}
}
