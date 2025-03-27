package v1

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/yusing/go-proxy/internal/homepage"
	"github.com/yusing/go-proxy/internal/net/gphttp"
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
	}
	HomepageOverrideItemsBatchParams struct {
		Value map[string]*homepage.ItemConfig `json:"value"`
	}
	HomepageOverrideCategoryOrderParams struct {
		Which string `json:"which"`
		Value int    `json:"value"`
	}
	HomepageOverrideItemVisibleParams struct {
		Which []string `json:"which"`
		Value bool     `json:"value"`
	}
)

func SetHomePageOverrides(w http.ResponseWriter, r *http.Request) {
	what := r.FormValue("what")
	if what == "" {
		gphttp.BadRequest(w, "missing what or which")
		return
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		gphttp.ClientError(w, err, http.StatusBadRequest)
		return
	}
	r.Body.Close()

	overrides := homepage.GetOverrideConfig()
	switch what {
	case HomepageOverrideItem:
		var params HomepageOverrideItemParams
		if err := json.Unmarshal(data, &params); err != nil {
			gphttp.ClientError(w, err, http.StatusBadRequest)
			return
		}
		overrides.OverrideItem(params.Which, &params.Value)
	case HomepageOverrideItemsBatch:
		var params HomepageOverrideItemsBatchParams
		if err := json.Unmarshal(data, &params); err != nil {
			gphttp.ClientError(w, err, http.StatusBadRequest)
			return
		}
		overrides.OverrideItems(params.Value)
	case HomepageOverrideItemVisible: // POST /v1/item_visible [a,b,c], false => hide a, b, c
		var params HomepageOverrideItemVisibleParams
		if err := json.Unmarshal(data, &params); err != nil {
			gphttp.ClientError(w, err, http.StatusBadRequest)
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
			gphttp.ClientError(w, err, http.StatusBadRequest)
			return
		}
		overrides.SetCategoryOrder(params.Which, params.Value)
	default:
		http.Error(w, "invalid what", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}
