package routeApi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	entrypoint "github.com/yusing/godoxy/internal/entrypoint/types"
	"github.com/yusing/godoxy/internal/route"

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
	ep := entrypoint.FromCtx(c.Request.Context())
	c.JSON(http.StatusOK, ep.RoutesByProvider())
}
