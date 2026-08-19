package homepage_test

import (
	"testing"

	. "github.com/yusing/godoxy/internal/homepage"
	"github.com/yusing/godoxy/internal/homepage/icons"

	expect "github.com/yusing/goutils/testing"
)

func TestOverrideItem(t *testing.T) {
	a := &Item{
		Alias: "foo",
		ItemConfig: ItemConfig{
			Show: false,
			Name: "Foo",
			Icon: &icons.URL{
				FullURL: new("/favicon.ico"),
				Source:  icons.SourceRelative,
			},
			Category: "App",
		},
	}
	want := ItemConfig{
		Show:     true,
		Name:     "Bar",
		Category: "Test",
		Icon: &icons.URL{
			FullURL: new("@walkxcode/example.png"),
			Source:  icons.SourceWalkXCode,
		},
	}
	overrides := GetOverrideConfig()
	overrides.Initialize()
	overrides.OverrideItem(a.Alias, want)
	got := a.GetOverride()
	expect.Equal(t, got, Item{
		ItemConfig: want,
		Alias:      a.Alias,
	})
}

func TestOverrideItem_PreservesURL(t *testing.T) {
	a := &Item{
		Alias: "svc",
		ItemConfig: ItemConfig{
			Show: true,
			Name: "Service",
			URL:  "http://origin.local",
		},
	}
	wantCfg := ItemConfig{
		Show: true,
		Name: "Overridden",
		URL:  "http://should-not-apply",
	}
	overrides := GetOverrideConfig()
	overrides.Initialize()
	overrides.OverrideItem(a.Alias, wantCfg)

	got := a.GetOverride()
	expect.Equal(t, got.URL, "http://origin.local")
	expect.Equal(t, got.Name, "Overridden")
}

func TestVisibilityFavoriteAndSortOrders(t *testing.T) {
	a := &Item{
		Alias: "alpha",
		ItemConfig: ItemConfig{
			Show:     true,
			Name:     "Alpha",
			Category: "Apps",
			Favorite: false,
		},
	}
	overrides := GetOverrideConfig()
	overrides.Initialize()
	overrides.SetItemsVisibility([]string{a.Alias}, false)
	overrides.SetItemsFavorite([]string{a.Alias}, true)
	overrides.SetSortOrder(a.Alias, 5)
	overrides.SetAllSortOrder(a.Alias, 9)
	overrides.SetFavSortOrder(a.Alias, 2)

	got := a.GetOverride()
	expect.Equal(t, got.Show, false)
	expect.Equal(t, got.Favorite, true)
	expect.Equal(t, got.SortOrder, 5)
	expect.Equal(t, got.AllSortOrder, 9)
	expect.Equal(t, got.FavSortOrder, 2)
}

func TestCategoryDefaultedWhenEmpty(t *testing.T) {
	a := &Item{
		Alias: "no-cat",
		ItemConfig: ItemConfig{
			Show: true,
			Name: "NoCat",
		},
	}
	got := a.GetOverride()
	expect.Equal(t, got.Category, CategoryOthers)
}

func TestOverrideItems_Bulk(t *testing.T) {
	a := &Item{
		Alias: "bulk-1",
		ItemConfig: ItemConfig{
			Show:     true,
			Name:     "Bulk1",
			Category: "X",
		},
	}
	b := &Item{
		Alias: "bulk-2",
		ItemConfig: ItemConfig{
			Show:     true,
			Name:     "Bulk2",
			Category: "Y",
		},
	}

	overrides := GetOverrideConfig()
	overrides.Initialize()
	overrides.OverrideItems(map[string]ItemConfig{
		a.Alias: {Show: true, Name: "A*", Category: "AX"},
		b.Alias: {Show: false, Name: "B*", Category: "BY"},
	})

	ga := a.GetOverride()
	gb := b.GetOverride()

	expect.Equal(t, ga.Name, "A*")
	expect.Equal(t, ga.Category, "AX")
	expect.Equal(t, gb.Name, "B*")
	expect.Equal(t, gb.Category, "BY")
	expect.Equal(t, gb.Show, false)
}
