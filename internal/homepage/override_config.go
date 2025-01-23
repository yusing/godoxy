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
	DisplayOrder   map[string]int         `json:"display_order"` // TODO: implement this
	CategoryName   map[string]string      `json:"category_name"`
	CategoryOrder  map[string]int         `json:"category_order"` // TODO: implement this
	ItemVisibility map[string]bool        `json:"item_visibility"`
	mu             sync.RWMutex
}

var overrideConfigInstance *OverrideConfig

func must(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}

func InitOverridesConfig() {
	overrideConfigInstance = &OverrideConfig{
		ItemOverrides:  make(map[string]*ItemConfig),
		DisplayOrder:   make(map[string]int),
		CategoryName:   make(map[string]string),
		CategoryOrder:  make(map[string]int),
		ItemVisibility: make(map[string]bool),
	}
	err := utils.LoadJSONIfExist(common.HomepageJSONConfigPath, overrideConfigInstance)
	if err != nil {
		logging.Error().Err(err).Msg("failed to load homepage overrides config")
	} else {
		logging.Info().Msgf("homepage overrides config loaded, %d items", len(overrideConfigInstance.ItemOverrides))
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

func (c *OverrideConfig) UnmarshalJSON(data []byte) error {
	return utils.DeserializeJSON(data, c)
}

func (c *OverrideConfig) OverrideItem(alias string, override *ItemConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ItemOverrides[alias] = override
}

func (c *OverrideConfig) GetOverride(item *Item) *Item {
	orig := item
	c.mu.RLock()
	defer c.mu.RUnlock()
	itemOverride, ok := c.ItemOverrides[item.Alias]
	if !ok {
		if catOverride, ok := c.CategoryName[item.Category]; ok {
			clone := *item
			clone.Category = catOverride
			clone.IsUnset = false
			item = &clone
		}
	} else {
		clone := *item
		clone.ItemConfig = itemOverride
		clone.IsUnset = false
		if catOverride, ok := c.CategoryName[clone.Category]; ok {
			clone.Category = catOverride
		}
		item = &clone
	}
	if show, ok := c.ItemVisibility[item.Alias]; ok {
		if item == orig {
			clone := *item
			clone.Show = show
			item = &clone
		} else {
			item.Show = show
		}
	}
	return item
}

func (c *OverrideConfig) SetCategoryNameOverride(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.CategoryName[key] = value
}

func (c *OverrideConfig) SetCategoryOrder(key string, value int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.CategoryOrder[key] = value
}

func (c *OverrideConfig) UnhideItems(keys ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, key := range keys {
		c.ItemVisibility[key] = true
	}
}

func (c *OverrideConfig) HideItems(keys ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, key := range keys {
		c.ItemVisibility[key] = false
	}
}
