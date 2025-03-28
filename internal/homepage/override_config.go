package homepage

import (
	"sync"

	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils"
)

type OverrideConfig struct {
	ItemOverrides  map[string]*ItemConfig `json:"item_overrides"`
	DisplayOrder   map[string]int         `json:"display_order"`  // TODO: implement this
	CategoryOrder  map[string]int         `json:"category_order"` // TODO: implement this
	ItemVisibility map[string]bool        `json:"item_visibility"`
	mu             sync.RWMutex
}

var overrideConfigInstance = &OverrideConfig{
	ItemOverrides:  make(map[string]*ItemConfig),
	DisplayOrder:   make(map[string]int),
	CategoryOrder:  make(map[string]int),
	ItemVisibility: make(map[string]bool),
}

func InitOverridesConfig() {
	overrideConfigInstance.mu.Lock()
	defer overrideConfigInstance.mu.Unlock()

	err := utils.LoadJSONIfExist(common.HomepageJSONConfigPath, overrideConfigInstance)
	if err != nil {
		logging.Error().Err(err).Msg("failed to load homepage overrides config")
	} else if len(overrideConfigInstance.ItemOverrides) > 0 {
		logging.Info().
			Int("count", len(overrideConfigInstance.ItemOverrides)).
			Msg("homepage overrides config loaded")
	}
	task.OnProgramExit("save_homepage_json_config", func() {
		if len(overrideConfigInstance.ItemOverrides) == 0 {
			return
		}
		if err := utils.SaveJSON(common.HomepageJSONConfigPath, overrideConfigInstance, 0o644); err != nil {
			logging.Error().Err(err).Msg("failed to save homepage overrides config")
		}
	})
}

func GetOverrideConfig() *OverrideConfig {
	return overrideConfigInstance
}

func (c *OverrideConfig) OverrideItem(alias string, override *ItemConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ItemOverrides[alias] = override
}

func (c *OverrideConfig) OverrideItems(items map[string]*ItemConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, value := range items {
		c.ItemOverrides[key] = value
	}
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
