package routeApi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/route/routes"
	apitypes "github.com/yusing/goutils/apitypes"
)

type ListRouteRequest struct {
	Which string `uri:"which" validate:"required"`
} //	@name	ListRouteRequest

// @x-id				"route"
// @BasePath		/api/v1
// @Summary		List route
// @Description	List route
// @Tags			route
// @Accept			json
// @Produce		json
// @Param			which	path		string	true	"Route name"
// @Success		200		{object}	RouteType
// @Failure		400		{object}	apitypes.ErrorResponse
// @Failure		403		{object}	apitypes.ErrorResponse
// @Failure		404		{object}	apitypes.ErrorResponse
// @Router			/route/{which} [get]
func Route(c *gin.Context) {
	var request ListRouteRequest
	if err := c.ShouldBindUri(&request); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}

	route, ok := routes.GetIncludeExcluded(request.Which)
	if ok {
		c.JSON(http.StatusOK, route)
		return
	}
	c.JSON(http.StatusNotFound, nil)
}
