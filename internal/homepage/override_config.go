package homepage

import (
	"maps"
	"sync"

	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/jsonstore"
)

type OverrideConfig struct {
	ItemOverrides  map[string]*ItemConfig `json:"item_overrides"`
	DisplayOrder   map[string]int         `json:"display_order"`  // TODO: implement this
	CategoryOrder  map[string]int         `json:"category_order"` // TODO: implement this
	ItemVisibility map[string]bool        `json:"item_visibility"`
	mu             sync.RWMutex
}

var overrideConfigInstance = jsonstore.Object[*OverrideConfig](common.NamespaceHomepageOverrides)

func GetOverrideConfig() *OverrideConfig {
	return overrideConfigInstance
}

func (c *OverrideConfig) Initialize() {
	c.ItemOverrides = make(map[string]*ItemConfig)
	c.DisplayOrder = make(map[string]int)
	c.CategoryOrder = make(map[string]int)
	c.ItemVisibility = make(map[string]bool)
}

func (c *OverrideConfig) OverrideItem(alias string, override *ItemConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ItemOverrides[alias] = override
}

func (c *OverrideConfig) OverrideItems(items map[string]*ItemConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	maps.Copy(c.ItemOverrides, items)
}

func (c *OverrideConfig) GetOverride(alias string, item *ItemConfig) *ItemConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	itemOverride, hasOverride := c.ItemOverrides[alias]
	if hasOverride {
		return itemOverride
	}
	if show, ok := c.ItemVisibility[alias]; ok {
		clone := *item
		clone.Show = show
		return &clone
	}
	return item
}

func (c *OverrideConfig) SetCategoryOrder(key string, value int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.CategoryOrder[key] = value
}

func (c *OverrideConfig) UnhideItems(keys []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, key := range keys {
		c.ItemVisibility[key] = true
	}
}

func (c *OverrideConfig) HideItems(keys []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, key := range keys {
		c.ItemVisibility[key] = false
	}
}
