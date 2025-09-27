package v1

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/route/routes"
	"github.com/yusing/goutils/http/httpheaders"
	"github.com/yusing/goutils/http/websocket"
)

type HealthMap = map[string]routes.HealthInfo //	@name	HealthMap

// @x-id				"health"
// @BasePath		/api/v1
// @Summary		Get routes health info
// @Description	Get health info by route name
// @Tags			v1,websocket
// @Accept			json
// @Produce		json
// @Success		200	{object}	HealthMap "Health info by route name"
// @Failure		403	{object}	apitypes.ErrorResponse
// @Failure		500	{object}	apitypes.ErrorResponse
// @Router			/health [get]
func Health(c *gin.Context) {
	if httpheaders.IsWebsocket(c.Request.Header) {
		websocket.PeriodicWrite(c, 1*time.Second, func() (any, error) {
			return routes.GetHealthInfo(), nil
		})
	} else {
		c.JSON(http.StatusOK, routes.GetHealthInfo())
	}
}
