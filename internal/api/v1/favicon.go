package v1

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/homepage/icons"
	iconfetch "github.com/yusing/godoxy/internal/homepage/icons/fetch"
	"github.com/yusing/godoxy/internal/route/routes"
	apitypes "github.com/yusing/goutils/apitypes"

	_ "unsafe"
)

type GetFavIconRequest struct {
	URL     string        `form:"url" binding:"required_without=Alias"`
	Alias   string        `form:"alias" binding:"required_without=URL"`
	Variant icons.Variant `form:"variant" binding:"omitempty,oneof=light dark"`
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
// @Success		200		{array}		iconfetch.Result
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
		var iconURL icons.URL
		if err := iconURL.Parse(request.URL); err != nil {
			c.JSON(http.StatusBadRequest, apitypes.Error("invalid url", err))
			return
		}
		icon := &iconURL
		if request.Variant != icons.VariantNone {
			icon = icon.WithVariant(request.Variant)
		}
		fetchResult, err := iconfetch.FetchFavIconFromURL(c.Request.Context(), icon)
		if err != nil {
			iconfetch.GinError(c, fetchResult.StatusCode, err)
			return
		}
		c.Data(fetchResult.StatusCode, fetchResult.ContentType(), fetchResult.Icon)
		return
	}

	// try with alias
	result, err := GetFavIconFromAlias(c.Request.Context(), request.Alias, request.Variant)
	if err != nil {
		iconfetch.GinError(c, result.StatusCode, err)
		return
	}
	c.Data(result.StatusCode, result.ContentType(), result.Icon)
}

//go:linkname GetFavIconFromAlias v1.GetFavIconFromAlias
func GetFavIconFromAlias(ctx context.Context, alias string, variant icons.Variant) (iconfetch.Result, error) {
	// try with route.Icon
	r, ok := routes.HTTP.Get(alias)
	if !ok {
		return iconfetch.FetchResultWithErrorf(http.StatusNotFound, "route not found")
	}

	var (
		result iconfetch.Result
		err    error
	)
	hp := r.HomepageItem()
	if hp.Icon != nil {
		if hp.Icon.Source == icons.SourceRelative {
			result, err = iconfetch.FindIcon(ctx, r, *hp.Icon.FullURL, variant)
		} else if variant != icons.VariantNone {
			result, err = iconfetch.FetchFavIconFromURL(ctx, hp.Icon.WithVariant(variant))
			if err != nil {
				// fallback to no variant
				result, err = iconfetch.FetchFavIconFromURL(ctx, hp.Icon.WithVariant(icons.VariantNone))
			}
		} else {
			result, err = iconfetch.FetchFavIconFromURL(ctx, hp.Icon)
		}
	} else {
		// try extract from "link[rel=icon]"
		result, err = iconfetch.FindIcon(ctx, r, "/", variant)
	}
	if result.StatusCode == 0 {
		result.StatusCode = http.StatusOK
	}
	return result, err
}
