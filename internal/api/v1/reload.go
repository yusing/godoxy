package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/config"
	apitypes "github.com/yusing/goutils/apitypes"
)

// @x-id				"reload"
// @BasePath		/api/v1
// @Summary		Reload config
// @Description	Reload config
// @Tags			v1
// @Accept			json
// @Produce		json
// @Success		200	{object}	apitypes.SuccessResponse
// @Failure		403	{object}	apitypes.ErrorResponse
// @Failure		500	{object}	apitypes.ErrorResponse
// @Router			/reload [post]
func Reload(c *gin.Context) {
	if err := config.Reload(); err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to reload config"))
		return
	}
	c.JSON(http.StatusOK, apitypes.Success("config reloaded"))
}
