package homepage

import (
	"encoding/json"
	"strings"

	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/utils"
)

type (
	Homepage map[string]Category
	Category []*Item

	ItemConfig struct {
		Show         bool           `json:"show"`
		Name         string         `json:"name"` // display name
		Icon         *IconURL       `json:"icon"`
		Category     string         `json:"category"`
		Description  string         `json:"description" aliases:"desc"`
		SortOrder    int            `json:"sort_order"`
		WidgetConfig map[string]any `json:"widget_config" aliases:"widget"`
	}

	Item struct {
		*ItemConfig

		Alias    string
		Provider string
	}
)

func init() {
	utils.RegisterDefaultValueFactory(func() *ItemConfig {
		return &ItemConfig{
			Show: true,
		}
	})
}

func (cfg *ItemConfig) GetOverride(alias string) *ItemConfig {
	return overrideConfigInstance.GetOverride(alias, cfg)
}

func (item *Item) MarshalJSON() ([]byte, error) {
	var url *string
	if !strings.ContainsRune(item.Alias, '.') {
		godoxyCfg := config.GetInstance().Value()
		// use first domain as base domain
		domains := godoxyCfg.MatchDomains
		if len(domains) > 0 {
			url = new(string)
			*url = item.Alias + domains[0]
		}
	} else {
		url = &item.Alias
	}
	return json.Marshal(map[string]any{
		"show":          item.Show,
		"alias":         item.Alias,
		"provider":      item.Provider,
		"url":           url,
		"name":          item.Name,
		"icon":          item.Icon,
		"category":      item.Category,
		"description":   item.Description,
		"sort_order":    item.SortOrder,
		"widget_config": item.WidgetConfig,
	})
}

func (c Homepage) Add(item *Item) {
	if c[item.Category] == nil {
		c[item.Category] = make(Category, 0)
	}
	c[item.Category] = append(c[item.Category], item)
}
