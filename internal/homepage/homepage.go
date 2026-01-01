package homepage

import (
	"slices"
	"strings"

	"github.com/yusing/ds/ordered"
	"github.com/yusing/godoxy/internal/homepage/widgets"
	"github.com/yusing/godoxy/internal/serialization"
	strutils "github.com/yusing/goutils/strings"
)

type (
	HomepageMap struct {
		ordered.Map[string, *Category]
	} // @name HomepageItemsMap

	Homepage []*Category // @name HomepageItems
	Category struct {
		Items []*Item `json:"items"`
		Name  string  `json:"name"`
	} // @name HomepageCategory

	ItemConfig struct {
		Show        bool     `json:"show"`
		Name        string   `json:"name"` // display name
		Icon        *IconURL `json:"icon" swaggertype:"string"`
		Category    string   `json:"category" validate:"omitempty"`
		Description string   `json:"description" aliases:"desc"`
		URL         string   `json:"url,omitempty"`
		Favorite    bool     `json:"favorite"`

		WidgetConfig *widgets.Config `json:"widget_config,omitempty" aliases:"widget" extensions:"x-nullable"`
	} // @name HomepageItemConfig

	Widget struct {
		Label string `json:"label"`
		Value string `json:"value"`
	} // @name HomepageItemWidget

	Item struct {
		ItemConfig

		SortOrder    int `json:"sort_order"`     // sort order in category
		FavSortOrder int `json:"fav_sort_order"` // sort order in favorite
		AllSortOrder int `json:"all_sort_order"` // sort order in all

		Clicks int `json:"clicks"`

		Widgets []Widget `json:"widgets,omitempty"`

		Alias       string `json:"alias"`
		Provider    string `json:"provider"`
		OriginURL   string `json:"origin_url"`
		ContainerID string `json:"container_id,omitempty" extensions:"x-nullable"`
	} // @name HomepageItem

	SortMethod string // @name HomepageSortMethod
)

const (
	CategoryAll       = "All"
	CategoryFavorites = "Favorites"
	CategoryHidden    = "Hidden"
	CategoryOthers    = "Others"
)

const (
	SortMethodClicks       = "clicks"       // @name HomepageSortMethodClicks
	SortMethodAlphabetical = "alphabetical" // @name HomepageSortMethodAlphabetical
	SortMethodCustom       = "custom"       // @name HomepageSortMethodCustom
)

func init() {
	serialization.RegisterDefaultValueFactory(func() *ItemConfig {
		return &ItemConfig{
			Show: true,
		}
	})
}

func NewHomepageMap(total int) *HomepageMap {
	m := &HomepageMap{
		Map: *ordered.NewMap[string, *Category](ordered.WithCapacity(10)),
	}
	m.Set(CategoryFavorites, &Category{
		Items: make([]*Item, 0), // no capacity reserved for this category
		Name:  CategoryFavorites,
	})
	m.Set(CategoryAll, &Category{
		Items: make([]*Item, 0, total),
		Name:  CategoryAll,
	})
	m.Set(CategoryHidden, &Category{
		Items: make([]*Item, 0),
		Name:  CategoryHidden,
	})
	return m
}

func (cfg Item) GetOverride() Item {
	return overrideConfigInstance.GetOverride(cfg)
}

func (c *HomepageMap) Add(item *Item) {
	c.add(item, item.Category)
	// add to all category even if item is hidden
	c.add(item, CategoryAll)
	if item.Show {
		if item.Favorite {
			c.add(item, CategoryFavorites)
		}
	} else {
		c.add(item, CategoryHidden)
	}
}

func (c *HomepageMap) add(item *Item, categoryName string) {
	category := c.Get(categoryName)
	if category == nil {
		category = &Category{
			Items: make([]*Item, 0),
			Name:  categoryName,
		}
		c.Set(categoryName, category)
	}
	category.Items = append(category.Items, item)
}

func (c *Category) Sort(method SortMethod) {
	switch method {
	case SortMethodClicks:
		c.sortByClicks()
	case SortMethodAlphabetical:
		c.sortByAlphabetical()
	case SortMethodCustom:
		c.sortByCustom()
	}
}

func (c *Category) sortByClicks() {
	slices.SortStableFunc(c.Items, func(a, b *Item) int {
		if a.Clicks > b.Clicks {
			return -1
		}
		if a.Clicks < b.Clicks {
			return 1
		}
		// fallback to alphabetical
		return strings.Compare(strutils.Title(a.Name), strutils.Title(b.Name))
	})
}

func (c *Category) sortByAlphabetical() {
	slices.SortStableFunc(c.Items, func(a, b *Item) int {
		return strings.Compare(strutils.Title(a.Name), strutils.Title(b.Name))
	})
}

func (c *Category) sortByCustom() {
	switch c.Name {
	case CategoryFavorites:
		slices.SortStableFunc(c.Items, func(a, b *Item) int {
			if a.FavSortOrder < b.FavSortOrder {
				return -1
			}
			if a.FavSortOrder > b.FavSortOrder {
				return 1
			}
			return 0
		})
	case CategoryAll:
		slices.SortStableFunc(c.Items, func(a, b *Item) int {
			if a.AllSortOrder < b.AllSortOrder {
				return -1
			}
			if a.AllSortOrder > b.AllSortOrder {
				return 1
			}
			return 0
		})
	default:
		slices.SortStableFunc(c.Items, func(a, b *Item) int {
			if a.SortOrder < b.SortOrder {
				return -1
			}
			if a.SortOrder > b.SortOrder {
				return 1
			}
			return 0
		})
	}
}
