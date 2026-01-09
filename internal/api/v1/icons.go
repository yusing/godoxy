package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	iconlist "github.com/yusing/godoxy/internal/homepage/icons/list"
	apitypes "github.com/yusing/goutils/apitypes"
)

type ListIconsRequest struct {
	Limit   int    `form:"limit" validate:"omitempty,min=0"`
	Keyword string `form:"keyword" validate:"required"`
} //	@name	ListIconsRequest

// @x-id				"icons"
// @BasePath		/api/v1
// @Summary		List icons
// @Description	List icons
// @Tags			v1
// @Accept			json
// @Produce		json
// @Param			limit	query		int		false	"Limit"
// @Param			keyword	query		string	false	"Keyword"
// @Success		200		{array}		homepage.IconMetaSearch
// @Failure		400		{object}	apitypes.ErrorResponse
// @Failure		403		{object}	apitypes.ErrorResponse
// @Router			/icons [get]
func Icons(c *gin.Context) {
	var request ListIconsRequest
	if err := c.ShouldBindQuery(&request); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}
	icons := iconlist.SearchIcons(request.Keyword, request.Limit)
	c.JSON(http.StatusOK, icons)
}
