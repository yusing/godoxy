package routeApi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/route/routes"

	_ "github.com/yusing/goutils/apitypes"
)

type RoutesByProvider map[string][]route.Route

// @x-id				"byProvider"
// @BasePath		/api/v1
// @Summary		List routes by provider
// @Description	List routes by provider
// @Tags			route
// @Accept			json
// @Produce		json
// @Success		200	{object}	RoutesByProvider
// @Failure		403	{object}	apitypes.ErrorResponse
// @Failure		500	{object}	apitypes.ErrorResponse
// @Router			/route/by_provider [get]
func ByProvider(c *gin.Context) {
	c.JSON(http.StatusOK, routes.ByProvider())
}
