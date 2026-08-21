package iconlist

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	. "github.com/yusing/godoxy/internal/homepage/icons"
)

const testWalkxcodeIcons = `{
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

const testSelfhstIcons = `[
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
	Key Key
	Meta
}

func runTests(t *testing.T, iconsCache IconMap, test []testCases) {
	t.Helper()

	for _, item := range test {
		icon, ok := iconsCache.Load(item.Key)
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
	mockHTTPGet(t, []byte(testWalkxcodeIcons))
	m := NewIconMap()
	if err := UpdateWalkxCodeIcons(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	if m.Size() != 3 {
		t.Fatalf("expect 3 icons, got %d", m.Size())
	}
	test := []testCases{
		{
			Key:   NewKey(SourceWalkXCode, "app1"),
			SVG:   true,
			PNG:   true,
			WebP:  true,
			Light: true,
		},
		{
			Key:  NewKey(SourceWalkXCode, "app2"),
			PNG:  true,
			WebP: true,
		},
		{
			Key:  NewKey(SourceWalkXCode, "karakeep"),
			SVG:  true,
			PNG:  true,
			WebP: true,
			Dark: true,
		},
	}
	runTests(t, m, test)
}

func TestListSelfhstIcons(t *testing.T) {
	mockHTTPGet(t, []byte(testSelfhstIcons))
	m := NewIconMap()
	if err := UpdateSelfhstIcons(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	if m.Size() != 3 {
		t.Fatalf("expect 3 icons, got %d", m.Size())
	}
	test := []testCases{
		{
			Key:         NewKey(SourceSelfhSt, "2fauth"),
			SVG:         true,
			PNG:         true,
			WebP:        true,
			Light:       true,
			Dark:        true,
			DisplayName: "2FAuth",
		},
		{
			Key:         NewKey(SourceSelfhSt, "dittofeed"),
			PNG:         true,
			WebP:        true,
			DisplayName: "Dittofeed",
		},
		{
			Key:         NewKey(SourceSelfhSt, "ars-technica"),
			SVG:         true,
			PNG:         true,
			WebP:        true,
			Light:       true,
			Dark:        true,
			DisplayName: "Ars Technica",
			Tag:         "News",
		},
	}
	runTests(t, m, test)
}

func TestIconCacheRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "icons.json")
	key := NewKey(SourceSelfhSt, "immich")
	want := &Meta{SVG: true, PNG: true, Light: true}
	m := NewIconMap()
	m.Store(key, want)

	require.NoError(t, saveIconCache(path, m))

	got, migrated, err := loadIconCache(path)
	require.NoError(t, err)
	require.False(t, migrated)
	require.Equal(t, 1, got.Size())
	meta, ok := got.Load(key)
	require.True(t, ok)
	require.Equal(t, want, meta)
}

func TestLoadIconCacheAlwaysInitializesMap(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		migrated bool
		wantSize int
	}{
		{name: "empty object", data: `{}`},
		{name: "null", data: `null`},
		{
			name:     "legacy wrapper",
			data:     `{"Icons":{"selfhst/immich":{"SVG":true}},"LastUpdate":"2026-05-01T00:00:00Z"}`,
			migrated: true,
			wantSize: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "icons.json")
			require.NoError(t, os.WriteFile(path, []byte(tt.data), 0o600))

			m, migrated, err := loadIconCache(path)
			require.NoError(t, err)
			require.NotNil(t, m)
			require.Equal(t, tt.migrated, migrated)
			require.Equal(t, tt.wantSize, m.Size())
		})
	}
}

func TestLoadIconCacheMalformed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "icons.json")
	require.NoError(t, os.WriteFile(path, []byte(`{`), 0o600))

	m, migrated, err := loadIconCache(path)
	require.Error(t, err)
	require.NotNil(t, m)
	require.False(t, migrated)
	require.Zero(t, m.Size())
}

func TestExpandRefsDropsTrailingSegments(t *testing.T) {
	got := ExpandRefs([]string{"immich-machine-learning", "Sonarr_TV", "arcane"})
	want := []string{
		"immich-machine-learning",
		"immich-machine",
		"immich",
		"sonarr-tv",
		"sonarr",
		"arcane",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestSearchIconsMatchesSuffixedKeyword(t *testing.T) {
	m := NewIconMap()
	m.Store(NewKey(SourceSelfhSt, "immich"), &Meta{SVG: true, DisplayName: "Immich"})
	m.Store(NewKey(SourceWalkXCode, "unrelated"), &Meta{PNG: true})
	prev := iconsCache.Swap(m)
	t.Cleanup(func() { iconsCache.Store(prev) })

	results := SearchIcons("immich-server", 5)
	if len(results) == 0 {
		t.Fatal("expected immich icon for immich-server")
	}
	if results[0].Ref != "immich" {
		t.Fatalf("expected immich first, got %s", results[0].Ref)
	}
}

func TestSearchIconsMatchesIconReference(t *testing.T) {
	m := NewIconMap()
	m.Store(NewKey(SourceSelfhSt, "immich"), &Meta{SVG: true, Light: true, DisplayName: "Immich"})
	m.Store(NewKey(SourceWalkXCode, "immich"), &Meta{PNG: true})
	prev := iconsCache.Swap(m)
	t.Cleanup(func() { iconsCache.Store(prev) })

	keywords := []string{
		"@selfhst/immich",
		"@selfhst/immich.svg",
		"@selfhst/immich-light.svg",
		// partially typed or edited extensions still match the same icon
		"@selfhst/immich.",
		"@selfhst/immich.sv",
		"@selfhst/immich-light.we",
	}
	for _, keyword := range keywords {
		results := SearchIcons(keyword, 5)
		if len(results) != 1 {
			t.Fatalf("expected 1 icon for %q, got %d", keyword, len(results))
		}
		if results[0].Source != SourceSelfhSt || results[0].Ref != "immich" {
			t.Fatalf("unexpected icon for %q: %s/%s", keyword, results[0].Source, results[0].Ref)
		}
	}

	if results := SearchIcons("@unknown/immich", 5); len(results) != 0 {
		t.Fatalf("expected no icon for unknown source, got %d", len(results))
	}
}

func TestResolveImmichServer(t *testing.T) {
	m := NewIconMap()
	m.Store(NewKey(SourceSelfhSt, "immich"), &Meta{
		SVG:         true,
		DisplayName: "Immich",
		Tag:         "Media",
	})
	prev := iconsCache.Swap(m)
	t.Cleanup(func() { iconsCache.Store(prev) })

	u, meta, ok := Resolve([]string{"immich-server"})
	if !ok {
		t.Fatal("expected resolve to succeed")
	}
	if meta.DisplayName != "Immich" || meta.Tag != "Media" {
		t.Fatalf("unexpected meta: %+v", meta)
	}
	if u == nil || u.String() != "@selfhst/immich.svg" {
		t.Fatalf("unexpected icon %v", u)
	}
}

func TestResolveIgnoresHyphenSeparators(t *testing.T) {
	m := NewIconMap()
	m.Store(NewKey(SourceSelfhSt, "pocket-id"), &Meta{
		SVG:         true,
		DisplayName: "Pocket ID",
		Tag:         "Security",
	})
	prev := iconsCache.Swap(m)
	t.Cleanup(func() { iconsCache.Store(prev) })

	u, meta, ok := Resolve([]string{"pocketid"})
	if !ok {
		t.Fatal("expected resolve to succeed")
	}
	if meta.DisplayName != "Pocket ID" || meta.Tag != "Security" {
		t.Fatalf("unexpected meta: %+v", meta)
	}
	if u == nil || u.String() != "@selfhst/pocket-id.svg" {
		t.Fatalf("unexpected icon %v", u)
	}

	m.Store(NewKey(SourceSelfhSt, "pocketid"), &Meta{
		PNG:         true,
		DisplayName: "PocketID",
		Tag:         "Exact",
	})
	u, meta, ok = Resolve([]string{"pocketid"})
	if !ok || meta.DisplayName != "PocketID" || meta.Tag != "Exact" {
		t.Fatalf("expected exact metadata match, got %v %+v", ok, meta)
	}
	if u == nil || u.String() != "@selfhst/pocketid.png" {
		t.Fatalf("expected exact icon match, got %v", u)
	}
}

func TestResolveRejectsAmbiguousHyphenSeparatorMatch(t *testing.T) {
	m := NewIconMap()
	m.Store(NewKey(SourceSelfhSt, "pocket-id"), &Meta{SVG: true})
	m.Store(NewKey(SourceSelfhSt, "pocketi-d"), &Meta{SVG: true})
	prev := iconsCache.Swap(m)
	t.Cleanup(func() { iconsCache.Store(prev) })

	if u, meta, ok := Resolve([]string{"pocketid"}); ok {
		t.Fatalf("expected ambiguous match to fail, got %v %+v", u, meta)
	}
}

func TestResolvePrefersLongestExistingRef(t *testing.T) {
	m := NewIconMap()
	m.Store(NewKey(SourceSelfhSt, "sonarr"), &Meta{SVG: true, DisplayName: "Sonarr"})
	m.Store(NewKey(SourceSelfhSt, "home-assistant"), &Meta{PNG: true, DisplayName: "Home Assistant"})
	m.Store(NewKey(SourceSelfhSt, "home"), &Meta{PNG: true, DisplayName: "Home"})
	prev := iconsCache.Swap(m)
	t.Cleanup(func() { iconsCache.Store(prev) })

	u, meta, ok := Resolve([]string{"sonarr-tv"})
	if !ok || u == nil || u.String() != "@selfhst/sonarr.svg" || meta.DisplayName != "Sonarr" {
		t.Fatalf("unexpected resolve for sonarr-tv: %v %+v", u, meta)
	}

	u, meta, ok = Resolve([]string{"home-assistant"})
	if !ok || u == nil || u.String() != "@selfhst/home-assistant.png" || meta.DisplayName != "Home Assistant" {
		t.Fatalf("unexpected resolve for home-assistant: %v %+v", u, meta)
	}
}

func mockHTTPGet(tb testing.TB, body []byte) {
	tb.Helper()

	prev := httpGet
	httpGet = func(context.Context, string) ([]byte, func([]byte), error) {
		return body, func([]byte) {}, nil
	}
	tb.Cleanup(func() {
		httpGet = prev
	})
}
