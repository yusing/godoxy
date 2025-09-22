package routeApi

import (
	"net/http"
	"slices"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/net/gphttp/httpheaders"
	"github.com/yusing/godoxy/internal/net/gphttp/websocket"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/route/routes"
	"github.com/yusing/godoxy/internal/types"
)

type RouteType route.Route // @name Route

// @x-id				"routes"
// @BasePath		/api/v1
// @Summary		List routes
// @Description	List routes
// @Tags			route,websocket
// @Accept			json
// @Produce		json
// @Param			provider	query		string	false	"Provider"
// @Success		200			{array}		RouteType
// @Failure		403			{object}	apitypes.ErrorResponse
// @Router			/route/list [get]
func Routes(c *gin.Context) {
	if httpheaders.IsWebsocket(c.Request.Header) {
		RoutesWS(c)
		return
	}

	provider := c.Query("provider")
	if provider == "" {
		c.JSON(http.StatusOK, slices.Collect(routes.Iter))
		return
	}

	rts := make([]types.Route, 0, routes.NumRoutes())
	for r := range routes.Iter {
		if r.ProviderName() == provider {
			rts = append(rts, r)
		}
	}
	c.JSON(http.StatusOK, rts)
}

func RoutesWS(c *gin.Context) {
	provider := c.Query("provider")
	if provider == "" {
		websocket.PeriodicWrite(c, 3*time.Second, func() (any, error) {
			return slices.Collect(routes.Iter), nil
		})
		return
	}

	websocket.PeriodicWrite(c, 3*time.Second, func() (any, error) {
		rts := make([]types.Route, 0, routes.NumRoutes())
		for r := range routes.Iter {
			if r.ProviderName() == provider {
				rts = append(rts, r)
			}
		}
		return rts, nil
	})
}
