package homepageapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	entrypoint "github.com/yusing/godoxy/internal/entrypoint/types"
	"github.com/yusing/godoxy/internal/homepage"

	_ "github.com/yusing/goutils/apitypes"
	apitypes "github.com/yusing/goutils/apitypes"
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
// @Failure		500	{object}	apitypes.ErrorResponse
// @Router			/homepage/categories [get]
func Categories(c *gin.Context) {
	ep := entrypoint.FromCtx(c.Request.Context())
	if ep == nil { // impossible, but just in case
		c.JSON(http.StatusInternalServerError, apitypes.Error("entrypoint not initialized"))
		return
	}
	c.JSON(http.StatusOK, HomepageCategories(ep))
}

func HomepageCategories(ep entrypoint.Entrypoint) []string {
	check := make(map[string]struct{})
	categories := make([]string, 0)
	categories = append(categories, homepage.CategoryAll)
	categories = append(categories, homepage.CategoryFavorites)
	for _, r := range ep.HTTPRoutes().Iter {
		item := r.HomepageItem()
		if item.Category == "" {
			continue
		}
		if _, ok := check[item.Category]; ok {
			continue
		}
		check[item.Category] = struct{}{}
		categories = append(categories, item.Category)
	}
	return categories
}
