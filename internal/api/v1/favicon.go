package v1

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	apitypes "github.com/yusing/go-proxy/internal/api/types"
	"github.com/yusing/go-proxy/internal/homepage"
	"github.com/yusing/go-proxy/internal/route/routes"
)

type GetFavIconRequest struct {
	URL   string `form:"url" binding:"required_without=Alias"`
	Alias string `form:"alias" binding:"required_without=URL"`
} //	@name	GetFavIconRequest

// @x-id				"favicon"
// @BasePath		/api/v1
// @Summary		Get favicon
// @Description	Get favicon
// @Tags			v1
// @Accept			json
// @Produce		image/svg+xml,image/x-icon,image/png,image/webp
// @Param			url		query		string	false	"URL of the route"
// @Param			alias	query		string	false	"Alias of the route"
// @Success		200		{array}		homepage.FetchResult
// @Failure		400		{object}	apitypes.ErrorResponse "Bad Request: alias is empty or route is not HTTPRoute"
// @Failure		403		{object}	apitypes.ErrorResponse "Forbidden: unauthorized"
// @Failure		404		{object}	apitypes.ErrorResponse "Not Found: route or icon not found"
// @Failure		500		{object}	apitypes.ErrorResponse "Internal Server Error: internal error"
// @Router			/favicon [get]
func FavIcon(c *gin.Context) {
	var request GetFavIconRequest
	if err := c.ShouldBindQuery(&request); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}

	// try with url
	if request.URL != "" {
		var iconURL homepage.IconURL
		if err := iconURL.Parse(request.URL); err != nil {
			c.JSON(http.StatusBadRequest, apitypes.Error("invalid url", err))
			return
		}
		fetchResult := homepage.FetchFavIconFromURL(c.Request.Context(), &iconURL)
		if !fetchResult.OK() {
			c.JSON(fetchResult.StatusCode, apitypes.Error(fetchResult.ErrMsg))
			return
		}
		c.Data(fetchResult.StatusCode, fetchResult.ContentType(), fetchResult.Icon)
		return
	}

	// try with alias
	result := GetFavIconFromAlias(c.Request.Context(), request.Alias)
	if !result.OK() {
		c.JSON(result.StatusCode, apitypes.Error(result.ErrMsg))
		return
	}
	c.Data(result.StatusCode, result.ContentType(), result.Icon)
}

func GetFavIconFromAlias(ctx context.Context, alias string) *homepage.FetchResult {
	// try with route.Icon
	r, ok := routes.HTTP.Get(alias)
	if !ok {
		return &homepage.FetchResult{
			StatusCode: http.StatusNotFound,
			ErrMsg:     "route not found",
		}
	}

	var result *homepage.FetchResult
	hp := r.HomepageItem()
	if hp.Icon != nil {
		if hp.Icon.IconSource == homepage.IconSourceRelative {
			result = homepage.FindIcon(ctx, r, *hp.Icon.FullURL)
		} else {
			result = homepage.FetchFavIconFromURL(ctx, hp.Icon)
		}
	} else {
		// try extract from "link[rel=icon]"
		result = homepage.FindIcon(ctx, r, "/")
	}
	if result.StatusCode == 0 {
		result.StatusCode = http.StatusOK
	}
	return result
}
