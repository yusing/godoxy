package homepageapi

import (
	"encoding/json"
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
		Value map[string]*homepage.ItemConfig `json:"value"`
	} //	@name	HomepageOverrideItemsBatchParams
	HomepageOverrideCategoryOrderParams struct {
		Which string `json:"which"`
		Value int    `json:"value"`
	} //	@name	HomepageOverrideCategoryOrderParams
	HomepageOverrideItemVisibleParams struct {
		Which []string `json:"which"`
		Value bool     `json:"value"`
	} //	@name	HomepageOverrideItemVisibleParams
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
	overrides.OverrideItem(params.Which, &params.Value)
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
		data, derr := c.GetRawData()
		if derr != nil {
			c.Error(apitypes.InternalServerError(derr, "failed to get raw data"))
			return
		}
		if uerr := json.Unmarshal(data, &params); uerr != nil {
			c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", uerr))
			return
		}
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
		data, derr := c.GetRawData()
		if derr != nil {
			c.Error(apitypes.InternalServerError(derr, "failed to get raw data"))
			return
		}
		if uerr := json.Unmarshal(data, &params); uerr != nil {
			c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", uerr))
			return
		}
	}
	overrides := homepage.GetOverrideConfig()
	if params.Value {
		overrides.UnhideItems(params.Which)
	} else {
		overrides.HideItems(params.Which)
	}
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
		data, derr := c.GetRawData()
		if derr != nil {
			c.Error(apitypes.InternalServerError(derr, "failed to get raw data"))
			return
		}
		if uerr := json.Unmarshal(data, &params); uerr != nil {
			c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", uerr))
			return
		}
	}
	overrides := homepage.GetOverrideConfig()
	overrides.SetCategoryOrder(params.Which, params.Value)
	c.JSON(http.StatusOK, apitypes.Success("success"))
}
