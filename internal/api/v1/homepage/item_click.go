package homepageapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	apitypes "github.com/yusing/go-proxy/internal/api/types"
	"github.com/yusing/go-proxy/internal/homepage"
)

type HomepageOverrideItemClickParams struct {
	Which string `form:"which" binding:"required"`
} //	@name	HomepageOverrideItemClickParams

// @x-id				"item-click"
// @BasePath		/api/v1
// @Summary		Increment item click
// @Description	Increment item click.
// @Tags			homepage
// @Accept		json
// @Produce		json
// @Param		request	query		HomepageOverrideItemClickParams	true	"Increment item click"
// @Success		200		{object}	apitypes.SuccessResponse
// @Failure		400		{object}	apitypes.ErrorResponse
// @Failure		500		{object}	apitypes.ErrorResponse
// @Router			/homepage/item_click [post]
func ItemClick(c *gin.Context) {
	var params HomepageOverrideItemClickParams
	if err := c.ShouldBindQuery(&params); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}
	overrides := homepage.GetOverrideConfig()
	overrides.IncrementItemClicks(params.Which)
	c.JSON(http.StatusOK, apitypes.Success("success"))
}
