package v1

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	entrypoint "github.com/yusing/godoxy/internal/entrypoint/types"
	apitypes "github.com/yusing/goutils/apitypes"
	"github.com/yusing/goutils/http/httpheaders"
	"github.com/yusing/goutils/http/websocket"
)

// @x-id				"health"
// @BasePath		/api/v1
// @Summary		Get routes health info
// @Description	Get health info by route name
// @Tags			v1,websocket
// @Accept			json
// @Produce		json
// @Success		200	{object}	routes.HealthMap "Health info by route name"
// @Failure		403	{object}	apitypes.ErrorResponse
// @Failure		500	{object}	apitypes.ErrorResponse
// @Router			/health [get]
func Health(c *gin.Context) {
	ep := entrypoint.FromCtx(c.Request.Context())
	if ep == nil { // impossible, but just in case
		c.JSON(http.StatusInternalServerError, apitypes.Error("entrypoint not initialized"))
		return
	}
	if httpheaders.IsWebsocket(c.Request.Header) {
		websocket.PeriodicWrite(c, 1*time.Second, func() (any, error) {
			return ep.GetHealthInfoSimple(), nil
		})
	} else {
		c.JSON(http.StatusOK, ep.GetHealthInfoSimple())
	}
}
