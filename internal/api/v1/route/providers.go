package routeApi

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	config "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/net/gphttp/httpheaders"
	"github.com/yusing/godoxy/internal/net/gphttp/websocket"
)

// @x-id				"providers"
// @BasePath		/api/v1
// @Summary		List route providers
// @Description	List route providers
// @Tags			route,websocket
// @Accept			json
// @Produce		json
// @Success		200	{array}		config.RouteProviderListResponse
// @Failure		403	{object}	apitypes.ErrorResponse
// @Failure		500	{object}	apitypes.ErrorResponse
// @Router			/route/providers [get]
func Providers(c *gin.Context) {
	cfg := config.GetInstance()
	if httpheaders.IsWebsocket(c.Request.Header) {
		websocket.PeriodicWrite(c, 5*time.Second, func() (any, error) {
			return config.GetInstance().RouteProviderList(), nil
		})
	} else {
		c.JSON(http.StatusOK, cfg.RouteProviderList())
	}
}
