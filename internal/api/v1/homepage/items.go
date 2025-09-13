package homepageapi

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lithammer/fuzzysearch/fuzzy"
	apitypes "github.com/yusing/go-proxy/internal/api/types"
	"github.com/yusing/go-proxy/internal/homepage"
	"github.com/yusing/go-proxy/internal/net/gphttp/httpheaders"
	"github.com/yusing/go-proxy/internal/net/gphttp/websocket"
	"github.com/yusing/go-proxy/internal/route/routes"
)

type HomepageItemsRequest struct {
	SearchQuery string `form:"search"`   // Search query
	Category    string `form:"category"` // Category filter
	Provider    string `form:"provider"` // Provider filter
	// Sort method
	SortMethod homepage.SortMethod `form:"sort_method" default:"alphabetical" binding:"omitempty,oneof=clicks alphabetical custom"`
} //	@name	HomepageItemsRequest

// @x-id				"items"
// @BasePath		/api/v1
// @Summary		Homepage items
// @Description	Homepage items
// @Tags			homepage,websocket
// @Accept			json
// @Produce		json
// @Param			query		query		HomepageItemsRequest	false	"Query parameters"
// @Success		200			{object}	homepage.Homepage
// @Failure		400			{object}	apitypes.ErrorResponse
// @Failure		403			{object}	apitypes.ErrorResponse
// @Router			/homepage/items [get]
func Items(c *gin.Context) {
	var request HomepageItemsRequest
	if err := c.ShouldBindQuery(&request); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}

	proto := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		proto = "https"
	}
	hostname := c.Request.Host
	if host := c.GetHeader("X-Forwarded-Host"); host != "" {
		hostname = host
	}

	if httpheaders.IsWebsocket(c.Request.Header) {
		websocket.PeriodicWrite(c, 2*time.Second, func() (any, error) {
			return HomepageItems(proto, hostname, &request), nil
		})
	} else {
		c.JSON(http.StatusOK, HomepageItems(proto, hostname, &request))
	}
}

func HomepageItems(proto, hostname string, request *HomepageItemsRequest) homepage.Homepage {
	switch proto {
	case "http", "https":
	default:
		proto = "http"
	}

	hp := homepage.NewHomepageMap(routes.HTTP.Size())

	if strings.Count(hostname, ".") > 1 {
		_, hostname, _ = strings.Cut(hostname, ".") // remove the subdomain
	}

	for _, r := range routes.HTTP.Iter {
		if request.Provider != "" && r.ProviderName() != request.Provider {
			continue
		}
		item := r.HomepageItem()
		if request.Category != "" && item.Category != request.Category {
			continue
		}
		if request.SearchQuery != "" && !fuzzy.MatchFold(request.SearchQuery, item.Name) {
			continue
		}

		// clear url if invalid
		_, err := url.Parse(item.URL)
		if err != nil {
			item.URL = ""
		}

		// append hostname if provided and only if alias is not FQDN
		if hostname != "" && item.URL == "" {
			isFQDNAlias := strings.Contains(item.Alias, ".")
			if !isFQDNAlias {
				item.URL = fmt.Sprintf("%s://%s.%s", proto, item.Alias, hostname)
			} else {
				item.URL = fmt.Sprintf("%s://%s", proto, item.Alias)
			}
		}

		// prepend protocol if not exists
		if !strings.HasPrefix(item.URL, "http://") && !strings.HasPrefix(item.URL, "https://") {
			item.URL = fmt.Sprintf("%s://%s", proto, item.URL)
		}

		hp.Add(&item)
	}

	ret := hp.Values()
	// sort items in each category
	for _, category := range ret {
		category.Sort(request.SortMethod)
	}
	// sort categories
	overrides := homepage.GetOverrideConfig()
	slices.SortStableFunc(ret, func(a, b *homepage.Category) int {
		// if category is "Hidden", move it to the end of the list
		if a.Name == homepage.CategoryHidden {
			return 1
		}
		if b.Name == homepage.CategoryHidden {
			return -1
		}
		// sort categories by order in config
		return overrides.CategoryOrder[a.Name] - overrides.CategoryOrder[b.Name]
	})
	return ret
}
