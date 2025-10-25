package homepageapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/homepage"
	"github.com/yusing/godoxy/internal/route/routes"

	_ "github.com/yusing/goutils/apitypes"
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
	c.JSON(http.StatusOK, HomepageCategories())
}

func HomepageCategories() []string {
	check := make(map[string]struct{})
	categories := make([]string, 0)
	categories = append(categories, homepage.CategoryAll)
	categories = append(categories, homepage.CategoryFavorites)
	for _, r := range routes.HTTP.Iter {
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
