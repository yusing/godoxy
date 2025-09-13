package homepage

import (
	"maps"
	"sync"

	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/jsonstore"
)

type OverrideConfig struct {
	ItemOverrides  map[string]ItemConfig `json:"item_overrides"`
	DisplayOrder   map[string]int        `json:"display_order"`
	CategoryOrder  map[string]int        `json:"category_order"`
	AllSortOrder   map[string]int        `json:"all_sort_order"`
	FavSortOrder   map[string]int        `json:"fav_sort_order"`
	ItemClicks     map[string]int        `json:"item_clicks"`
	ItemVisibility map[string]bool       `json:"item_visibility"`
	ItemFavorite   map[string]bool       `json:"item_favorite"`
	mu             sync.RWMutex
}

var overrideConfigInstance = jsonstore.Object[*OverrideConfig](common.NamespaceHomepageOverrides)

func GetOverrideConfig() *OverrideConfig {
	return overrideConfigInstance
}

func (c *OverrideConfig) Initialize() {
	c.ItemOverrides = make(map[string]ItemConfig)
	c.DisplayOrder = make(map[string]int)
	c.CategoryOrder = make(map[string]int)
	c.AllSortOrder = make(map[string]int)
	c.FavSortOrder = make(map[string]int)
	c.ItemClicks = make(map[string]int)
	c.ItemVisibility = make(map[string]bool)
	c.ItemFavorite = make(map[string]bool)
}

func (c *OverrideConfig) OverrideItem(alias string, override ItemConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ItemOverrides[alias] = override
}

func (c *OverrideConfig) OverrideItems(items map[string]ItemConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	maps.Copy(c.ItemOverrides, items)
}

func (c *OverrideConfig) GetOverride(item Item) Item {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if overrides, hasOverride := c.ItemOverrides[item.Alias]; hasOverride {
		overrides.URL = item.URL // NOTE: we don't want to override the URL
		item.ItemConfig = overrides
	}

	if show, ok := c.ItemVisibility[item.Alias]; ok {
		item.Show = show
	}
	if fav, ok := c.ItemFavorite[item.Alias]; ok {
		item.Favorite = fav
	}
	if displayOrder, ok := c.DisplayOrder[item.Alias]; ok {
		item.SortOrder = displayOrder
	}
	if allSortOrder, ok := c.AllSortOrder[item.Alias]; ok {
		item.AllSortOrder = allSortOrder
	}
	if favSortOrder, ok := c.FavSortOrder[item.Alias]; ok {
		item.FavSortOrder = favSortOrder
	}
	if clicks, ok := c.ItemClicks[item.Alias]; ok {
		item.Clicks = clicks
	}

	if item.Category == "" {
		item.Category = CategoryOthers
	}
	return item
}

func (c *OverrideConfig) SetSortOrder(key string, value int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.DisplayOrder[key] = value
}

func (c *OverrideConfig) SetAllSortOrder(key string, value int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.AllSortOrder[key] = value
}

func (c *OverrideConfig) SetFavSortOrder(key string, value int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.FavSortOrder[key] = value
}

func (c *OverrideConfig) SetItemsVisibility(keys []string, value bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, key := range keys {
		c.ItemVisibility[key] = value
	}
}

func (c *OverrideConfig) SetItemsFavorite(keys []string, value bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, key := range keys {
		c.ItemFavorite[key] = value
	}
}

func (c *OverrideConfig) SetCategoryOrder(key string, value int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.CategoryOrder[key] = value
}

func (c *OverrideConfig) IncrementItemClicks(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ItemClicks[key]++
}
