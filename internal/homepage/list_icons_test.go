package homepage_test

import (
	"testing"

	. "github.com/yusing/godoxy/internal/homepage"
)

const walkxcodeIcons = `{
	"png": [
		"app1.png",
		"app1-light.png",
		"app2.png",
		"karakeep.png",
		"karakeep-dark.png"
	],
	"svg": [
		"app1.svg",
		"app1-light.svg",
		"karakeep.svg",
		"karakeep-dark.svg"
	],
	"webp": [
		"app1.webp",
		"app1-light.webp",
		"app2.webp",
		"karakeep.webp",
		"karakeep-dark.webp"
	]
}`

const selfhstIcons = `[
	{
			"Name": "2FAuth",
			"Reference": "2fauth",
			"SVG": "Yes",
			"PNG": "Yes",
			"WebP": "Yes",
			"Light": "Yes",
			"Dark": "Yes",
			"Category": "Self-Hosted",
			"Tags": "",
			"CreatedAt": "2024-08-16 00:27:23+00:00"
	},
	{
			"Name": "Dittofeed",
			"Reference": "dittofeed",
			"SVG": "No",
			"PNG": "Yes",
			"WebP": "Yes",
			"Light": "No",
			"Dark": "No",
			"Category": "Self-Hosted",
			"Tags": "",
			"CreatedAt": "2024-08-22 11:33:37+00:00"
	},
	{
			"Name": "Ars Technica",
			"Reference": "ars-technica",
			"SVG": "Yes",
			"PNG": "Yes",
			"WebP": "Yes",
			"Light": "Yes",
			"Dark": "Yes",
			"Category": "Other",
			"Tags": "News",
			"CreatedAt": "2025-04-09 11:15:01+00:00"
	}
]`

type testCases struct {
	Key IconKey
	IconMeta
}

func runTests(t *testing.T, iconsCache IconMap, test []testCases) {
	t.Helper()

	for _, item := range test {
		icon, ok := iconsCache[item.Key]
		if !ok {
			t.Fatalf("icon %s not found", item.Key)
		}
		if icon.PNG != item.PNG || icon.SVG != item.SVG || icon.WebP != item.WebP {
			t.Fatalf("icon %s file format mismatch", item.Key)
		}
		if icon.Light != item.Light || icon.Dark != item.Dark {
			t.Fatalf("icon %s variant mismatch", item.Key)
		}
		if icon.DisplayName != item.DisplayName {
			t.Fatalf("icon %s display name mismatch, expect %s, got %s", item.Key, item.DisplayName, icon.DisplayName)
		}
		if icon.Tag != item.Tag {
			t.Fatalf("icon %s tag mismatch, expect %s, got %s", item.Key, item.Tag, icon.Tag)
		}
	}
}

func TestListWalkxCodeIcons(t *testing.T) {
	t.Cleanup(TestClearIconsCache)

	MockHTTPGet([]byte(walkxcodeIcons))
	m := make(IconMap)
	if err := UpdateWalkxCodeIcons(m); err != nil {
		t.Fatal(err)
	}
	if len(m) != 3 {
		t.Fatalf("expect 3 icons, got %d", len(m))
	}
	test := []testCases{
		{
			Key: NewIconKey(IconSourceWalkXCode, "app1"),
			IconMeta: IconMeta{
				SVG:   true,
				PNG:   true,
				WebP:  true,
				Light: true,
			},
		},
		{
			Key: NewIconKey(IconSourceWalkXCode, "app2"),
			IconMeta: IconMeta{
				PNG:  true,
				WebP: true,
			},
		},
		{
			Key: NewIconKey(IconSourceWalkXCode, "karakeep"),
			IconMeta: IconMeta{
				SVG:  true,
				PNG:  true,
				WebP: true,
				Dark: true,
			},
		},
	}
	runTests(t, m, test)
}

func TestListSelfhstIcons(t *testing.T) {
	t.Cleanup(TestClearIconsCache)
	MockHTTPGet([]byte(selfhstIcons))
	m := make(IconMap)
	if err := UpdateSelfhstIcons(m); err != nil {
		t.Fatal(err)
	}
	if len(m) != 3 {
		t.Fatalf("expect 3 icons, got %d", len(m))
	}
	test := []testCases{
		{
			Key: NewIconKey(IconSourceSelfhSt, "2fauth"),
			IconMeta: IconMeta{
				SVG:         true,
				PNG:         true,
				WebP:        true,
				Light:       true,
				Dark:        true,
				DisplayName: "2FAuth",
			},
		},
		{
			Key: NewIconKey(IconSourceSelfhSt, "dittofeed"),
			IconMeta: IconMeta{
				PNG:         true,
				WebP:        true,
				DisplayName: "Dittofeed",
			},
		},
		{
			Key: NewIconKey(IconSourceSelfhSt, "ars-technica"),
			IconMeta: IconMeta{
				SVG:         true,
				PNG:         true,
				WebP:        true,
				Light:       true,
				Dark:        true,
				DisplayName: "Ars Technica",
				Tag:         "News",
			},
		},
	}
	runTests(t, m, test)
}
