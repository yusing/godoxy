package homepageapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	apitypes "github.com/yusing/go-proxy/internal/api/types"
	"github.com/yusing/go-proxy/internal/homepage"
)

type (
	HomepageOverrideItemParams struct {
		Which string              `json:"which"`
		Value homepage.ItemConfig `json:"value"`
	} //	@name	HomepageOverrideItemParams
	HomepageOverrideItemsBatchParams struct {
		Value map[string]homepage.ItemConfig `json:"value"`
	} //	@name	HomepageOverrideItemsBatchParams

	HomepageOverrideCategoryOrderParams struct {
		Which string `json:"which"`
		Value int    `json:"value"`
	} //	@name	HomepageOverrideCategoryOrderParams
	HomepageOverrideItemSortOrderParams    HomepageOverrideCategoryOrderParams //	@name	HomepageOverrideItemSortOrderParams
	HomepageOverrideItemAllSortOrderParams HomepageOverrideCategoryOrderParams //	@name	HomepageOverrideItemAllSortOrderParams
	HomepageOverrideItemFavSortOrderParams HomepageOverrideCategoryOrderParams //	@name	HomepageOverrideItemFavSortOrderParams

	HomepageOverrideItemVisibleParams struct {
		Which []string `json:"which"`
		Value bool     `json:"value"`
	} //	@name	HomepageOverrideItemVisibleParams
	HomepageOverrideItemFavoriteParams HomepageOverrideItemVisibleParams //	@name	HomepageOverrideItemFavoriteParams
)

// @x-id				"set-item"
// @BasePath		/api/v1
// @Summary		Override single homepage item
// @Description	Override single homepage item.
// @Tags			homepage
// @Accept		json
// @Produce		json
// @Param		request	body		HomepageOverrideItemParams	true	"Override single item"
// @Success		200		{object}	apitypes.SuccessResponse
// @Failure		400		{object}	apitypes.ErrorResponse
// @Failure		500		{object}	apitypes.ErrorResponse
// @Router			/homepage/set/item [post]
func SetItem(c *gin.Context) {
	var params HomepageOverrideItemParams
	if err := c.ShouldBindJSON(&params); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}
	overrides := homepage.GetOverrideConfig()
	overrides.OverrideItem(params.Which, params.Value)
	c.JSON(http.StatusOK, apitypes.Success("success"))
}

// @x-id				"set-items-batch"
// @BasePath		/api/v1
// @Summary		Override multiple homepage items
// @Description	Override multiple homepage items.
// @Tags			homepage
// @Accept		json
// @Produce		json
// @Param		request	body		HomepageOverrideItemsBatchParams	true	"Override multiple items"
// @Success		200		{object}	apitypes.SuccessResponse
// @Failure		400		{object}	apitypes.ErrorResponse
// @Failure		500		{object}	apitypes.ErrorResponse
// @Router			/homepage/set/items_batch [post]
func SetItemsBatch(c *gin.Context) {
	var params HomepageOverrideItemsBatchParams
	if err := c.ShouldBindJSON(&params); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}
	overrides := homepage.GetOverrideConfig()
	overrides.OverrideItems(params.Value)
	c.JSON(http.StatusOK, apitypes.Success("success"))
}

// @x-id				"set-item-visible"
// @BasePath		/api/v1
// @Summary		Set homepage item visibility
// @Description	POST list of item ids and visibility value.
// @Tags			homepage
// @Accept		json
// @Produce		json
// @Param		request	body		HomepageOverrideItemVisibleParams	true	"Set item visibility"
// @Success		200		{object}	apitypes.SuccessResponse
// @Failure		400		{object}	apitypes.ErrorResponse
// @Failure		500		{object}	apitypes.ErrorResponse
// @Router			/homepage/set/item_visible [post]
func SetItemVisible(c *gin.Context) {
	var params HomepageOverrideItemVisibleParams
	if err := c.ShouldBindJSON(&params); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}
	overrides := homepage.GetOverrideConfig()
	overrides.SetItemsVisibility(params.Which, params.Value)
	c.JSON(http.StatusOK, apitypes.Success("success"))
}

// @x-id				"set-item-favorite"
// @BasePath		/api/v1
// @Summary		Set homepage item favorite
// @Description	Set homepage item favorite.
// @Tags			homepage
// @Accept		json
// @Produce		json
// @Param		request	body		HomepageOverrideItemFavoriteParams	true	"Set item favorite"
// @Success		200		{object}	apitypes.SuccessResponse
// @Failure		400		{object}	apitypes.ErrorResponse
// @Failure		500		{object}	apitypes.ErrorResponse
// @Router			/homepage/set/item_favorite [post]
func SetItemFavorite(c *gin.Context) {
	var params HomepageOverrideItemFavoriteParams
	if err := c.ShouldBindJSON(&params); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}
	overrides := homepage.GetOverrideConfig()
	overrides.SetItemsFavorite(params.Which, params.Value)
	c.JSON(http.StatusOK, apitypes.Success("success"))
}

// @x-id				"set-item-sort-order"
// @BasePath		/api/v1
// @Summary		Set homepage item sort order
// @Description	Set homepage item sort order.
// @Tags			homepage
// @Accept		json
// @Produce		json
// @Param		request	body		HomepageOverrideItemSortOrderParams	true	"Set item sort order"
// @Success		200		{object}	apitypes.SuccessResponse
// @Failure		400		{object}	apitypes.ErrorResponse
// @Failure		500		{object}	apitypes.ErrorResponse
// @Router			/homepage/set/item_sort_order [post]
func SetItemSortOrder(c *gin.Context) {
	var params HomepageOverrideItemSortOrderParams
	if err := c.ShouldBindJSON(&params); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}
	overrides := homepage.GetOverrideConfig()
	overrides.SetSortOrder(params.Which, params.Value)
	c.JSON(http.StatusOK, apitypes.Success("success"))
}

// @x-id				"set-item-all-sort-order"
// @BasePath		/api/v1
// @Summary		Set homepage item all sort order
// @Description	Set homepage item all sort order.
// @Tags			homepage
// @Accept		json
// @Produce		json
// @Param		request	body		HomepageOverrideItemAllSortOrderParams	true	"Set item all sort order"
// @Success		200		{object}	apitypes.SuccessResponse
// @Failure		400		{object}	apitypes.ErrorResponse
// @Failure		500		{object}	apitypes.ErrorResponse
// @Router			/homepage/set/item_all_sort_order [post]
func SetItemAllSortOrder(c *gin.Context) {
	var params HomepageOverrideItemAllSortOrderParams
	if err := c.ShouldBindJSON(&params); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}
	overrides := homepage.GetOverrideConfig()
	overrides.SetAllSortOrder(params.Which, params.Value)
	c.JSON(http.StatusOK, apitypes.Success("success"))
}

// @x-id				"set-item-fav-sort-order"
// @BasePath		/api/v1
// @Summary		Set homepage item fav sort order
// @Description	Set homepage item fav sort order.
// @Tags			homepage
// @Accept		json
// @Produce		json
// @Param		request	body		HomepageOverrideItemFavSortOrderParams	true	"Set item fav sort order"
// @Success		200		{object}	apitypes.SuccessResponse
// @Failure		400		{object}	apitypes.ErrorResponse
// @Failure		500		{object}	apitypes.ErrorResponse
// @Router			/homepage/set/item_fav_sort_order [post]
func SetItemFavSortOrder(c *gin.Context) {
	var params HomepageOverrideItemFavSortOrderParams
	if err := c.ShouldBindJSON(&params); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}
	overrides := homepage.GetOverrideConfig()
	overrides.SetFavSortOrder(params.Which, params.Value)
	c.JSON(http.StatusOK, apitypes.Success("success"))
}

// @x-id				"set-category-order"
// @BasePath		/api/v1
// @Summary		Set homepage category order
// @Description	Set homepage category order.
// @Tags			homepage
// @Accept		json
// @Produce		json
// @Param		request	body		HomepageOverrideCategoryOrderParams	true	"Override category order"
// @Success		200		{object}	apitypes.SuccessResponse
// @Failure		400		{object}	apitypes.ErrorResponse
// @Failure		500		{object}	apitypes.ErrorResponse
// @Router			/homepage/set/category_order [post]
func SetCategoryOrder(c *gin.Context) {
	var params HomepageOverrideCategoryOrderParams
	if err := c.ShouldBindJSON(&params); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}
	overrides := homepage.GetOverrideConfig()
	overrides.SetCategoryOrder(params.Which, params.Value)
	c.JSON(http.StatusOK, apitypes.Success("success"))
}
