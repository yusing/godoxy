package homepageapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	apitypes "github.com/yusing/go-proxy/internal/api/types"
	"github.com/yusing/go-proxy/internal/route/routes"
)

type HomepageItemsRequest struct {
	Category string `form:"category" validate:"omitempty"`
	Provider string `form:"provider" validate:"omitempty"`
} //	@name	HomepageItemsRequest

// @x-id				"items"
// @BasePath		/api/v1
// @Summary		Homepage items
// @Description	Homepage items
// @Tags			homepage
// @Accept			json
// @Produce		json
// @Param			category	query		string	false	"Category filter"
// @Param			provider	query		string	false	"Provider filter"
// @Success		200			{object}	homepage.Homepage
// @Failure		400			{object}	apitypes.ErrorResponse
// @Failure		403			{object}	apitypes.ErrorResponse
// @Router			/homepage/items [get]
func Items(c *gin.Context) {
	var request HomepageItemsRequest
	if err := c.ShouldBindQuery(&request); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}

	proto := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		proto = "https"
	}
	hostname := c.Request.Host
	if host := c.GetHeader("X-Forwarded-Host"); host != "" {
		hostname = host
	}

	c.JSON(http.StatusOK, routes.HomepageItems(proto, hostname, request.Category, request.Provider))
}
