package homepageapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	apitypes "github.com/yusing/go-proxy/internal/api/types"
	"github.com/yusing/go-proxy/internal/homepage"
)

const (
	HomepageOverrideItem          = "item"
	HomepageOverrideItemsBatch    = "items_batch"
	HomepageOverrideCategoryOrder = "category_order"
	HomepageOverrideItemVisible   = "item_visible"
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

type SetHomePageOverridesRequest struct {
	What  string `json:"what" validate:"required,oneof=item items_batch category_order item_visible"`
	Value any    `json:"value" validate:"required" swaggerType:"object"`
}

// @x-id				"set"
// @BasePath		/api/v1
// @Summary		Set homepage overrides
// @Description	Set homepage overrides
// @Tags			homepage
// @Accept			json
// @Produce		json
// @Param			request	body		SetHomePageOverridesRequest{value=HomepageOverrideItemParams}			true	"Override single item"
// @Param			request	body		SetHomePageOverridesRequest{value=HomepageOverrideItemsBatchParams}		true	"Override multiple items"
// @Param			request	body		SetHomePageOverridesRequest{value=HomepageOverrideCategoryOrderParams}	true	"Override category order"
// @Param			request	body		SetHomePageOverridesRequest{value=HomepageOverrideItemVisibleParams}	true	"Override item visibility"
// @Success		200		{object}	apitypes.SuccessResponse
// @Failure		400		{object}	apitypes.ErrorResponse
// @Failure		500		{object}	apitypes.ErrorResponse
// @Router			/homepage/set [post]
func Set(c *gin.Context) {
	var request SetHomePageOverridesRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}

	data, err := c.GetRawData()
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to get raw data"))
		return
	}

	overrides := homepage.GetOverrideConfig()
	switch request.What {
	case HomepageOverrideItem:
		var params HomepageOverrideItemParams
		if err := json.Unmarshal(data, &params); err != nil {
			c.Error(apitypes.InternalServerError(err, "failed to unmarshal data"))
			return
		}
		overrides.OverrideItem(params.Which, &params.Value)
	case HomepageOverrideItemsBatch:
		var params HomepageOverrideItemsBatchParams
		if err := json.Unmarshal(data, &params); err != nil {
			c.Error(apitypes.InternalServerError(err, "failed to unmarshal data"))
			return
		}
		overrides.OverrideItems(params.Value)
	case HomepageOverrideItemVisible: // POST /v1/item_visible [a,b,c], false => hide a, b, c
		var params HomepageOverrideItemVisibleParams
		if err := json.Unmarshal(data, &params); err != nil {
			c.Error(apitypes.InternalServerError(err, "failed to unmarshal data"))
			return
		}
		if params.Value {
			overrides.UnhideItems(params.Which)
		} else {
			overrides.HideItems(params.Which)
		}
	case HomepageOverrideCategoryOrder:
		var params HomepageOverrideCategoryOrderParams
		if err := json.Unmarshal(data, &params); err != nil {
			c.Error(apitypes.InternalServerError(err, "failed to unmarshal data"))
			return
		}
		overrides.SetCategoryOrder(params.Which, params.Value)
	default: // won't happen
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid what", errors.New("invalid what")))
		return
	}
	c.JSON(http.StatusOK, apitypes.Success("success"))
}
