package homepage_test

import (
	"testing"

	. "github.com/yusing/go-proxy/internal/homepage"
	. "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestOverrideItem(t *testing.T) {
	a := &Item{
		Alias: "foo",
		ItemConfig: &ItemConfig{
			Show: false,
			Name: "Foo",
			Icon: &IconURL{
				FullURL:    strPtr("/favicon.ico"),
				IconSource: IconSourceRelative,
			},
			Category: "App",
		},
	}
	want := &ItemConfig{
		Show:     true,
		Name:     "Bar",
		Category: "Test",
		Icon: &IconURL{
			FullURL:    strPtr("@walkxcode/example.png"),
			IconSource: IconSourceWalkXCode,
		},
	}
	overrides := GetOverrideConfig()
	overrides.OverrideItem(a.Alias, want)
	got := a.GetOverride(a.Alias)
	ExpectEqual(t, got, want)
}
