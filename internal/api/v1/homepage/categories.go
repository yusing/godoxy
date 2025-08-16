package homepageapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yusing/go-proxy/internal/route/routes"
)

// @x-id				"categories"
// @BasePath		/api/v1
// @Summary		List homepage categories
// @Description	List homepage categories
// @Tags			homepage
// @Accept			json
// @Produce		json
// @Success		200	{array}		string
// @Failure		403	{object}	apitypes.ErrorResponse
// @Router			/homepage/categories [get]
func Categories(c *gin.Context) {
	c.JSON(http.StatusOK, routes.HomepageCategories())
}
