package homepage

import (
	"testing"

	. "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestOverrideItem(t *testing.T) {
	InitOverridesConfig()
	a := &Item{
		Alias: "foo",
		ItemConfig: &ItemConfig{
			Show: false,
			Name: "Foo",
			Icon: &IconURL{
				Value:      "/favicon.ico",
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
			Value:      "@walkxcode/example.png",
			IconSource: IconSourceWalkXCode,
		},
	}
	overrides := GetOverrideConfig()
	overrides.OverrideItem(a.Alias, want)
	got := a.GetOverride(a.Alias)
	ExpectDeepEqual(t, got, want)
}
