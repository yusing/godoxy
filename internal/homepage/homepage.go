package homepage

import (
	"slices"

	"github.com/yusing/go-proxy/internal/homepage/widgets"
	"github.com/yusing/go-proxy/internal/serialization"
)

type (
	Homepage map[string]Category
	Category []*Item

	ItemConfig struct {
		Show        bool     `json:"show"`
		Name        string   `json:"name"` // display name
		Icon        *IconURL `json:"icon"`
		Category    string   `json:"category"`
		Description string   `json:"description" aliases:"desc"`
		URL         string   `json:"url,omitempty"`
		SortOrder   int      `json:"sort_order"`
	}

	Item struct {
		*ItemConfig
		WidgetConfig *widgets.Config `json:"widget_config,omitempty" aliases:"widget"`

		Alias     string `json:"alias"`
		Provider  string `json:"provider"`
		OriginURL string `json:"origin_url"`
	}
)

func init() {
	serialization.RegisterDefaultValueFactory(func() *ItemConfig {
		return &ItemConfig{
			Show: true,
		}
	})
}

func (cfg *ItemConfig) GetOverride(alias string) *ItemConfig {
	return overrideConfigInstance.GetOverride(alias, cfg)
}

func (c Homepage) Add(item *Item) {
	if c[item.Category] == nil {
		c[item.Category] = make(Category, 0)
	}
	c[item.Category] = append(c[item.Category], item)
	slices.SortStableFunc(c[item.Category], func(a, b *Item) int {
		if a.SortOrder < b.SortOrder {
			return -1
		}
		if a.SortOrder > b.SortOrder {
			return 1
		}
		return 0
	})
}
