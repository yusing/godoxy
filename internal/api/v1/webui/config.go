package webui

import (
	"net/http"

	"github.com/gin-gonic/gin"
	config "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/goutils/apitypes"
)

// @x-id			"config"
// @BasePath	/api/v1
// @Summary		Get WebUI config
// @Description	Get WebUI config
// @Tags			webui
// @Produce		json
// @Success		200			{object}  config.WebUIConfig "WebUI Config"
// @Failure		400			{object}	apitypes.ErrorResponse
// @Failure		403			{object}	apitypes.ErrorResponse
// @Failure		500			{object}	apitypes.ErrorResponse
// @Router		/webui/config [get]
func Config(c *gin.Context) {
	state := config.FromCtx(c.Request.Context())
	if state == nil {
		c.Error(apitypes.InternalServerError(nil, "no active config state"))
		return
	}

	value := state.Value()
	if value == nil {
		c.JSON(http.StatusOK, config.WebUIConfig{})
	} else {
		c.JSON(http.StatusOK, value.WebUI)
	}
}
