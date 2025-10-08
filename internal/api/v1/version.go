package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yusing/goutils/version"
)

// @x-id				"version"
// @BasePath		/api/v1
// @Summary		Get version
// @Description	Get the version of the GoDoxy
// @Tags			v1
// @Accept			json
// @Produce		plain
// @Success		200	{string}	string	"version"
// @Router			/version [get]
func Version(c *gin.Context) {
	c.JSON(http.StatusOK, version.Get().String())
}
