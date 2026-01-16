package iconlist

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/homepage/icons"
	"github.com/yusing/godoxy/internal/serialization"
	httputils "github.com/yusing/goutils/http"
	"github.com/yusing/goutils/intern"
	strutils "github.com/yusing/goutils/strings"
	"github.com/yusing/goutils/synk"
	"github.com/yusing/goutils/task"
)

type (
	IconMap  map[icons.Key]*icons.Meta
	IconList []string

	IconMetaSearch struct {
		*icons.Meta

		Source icons.Source `json:"Source"`
		Ref    string       `json:"Ref"`

		rank int
	} // @name IconMetaSearch
)

const updateInterval = 2 * time.Hour

var iconsCache synk.Value[IconMap]

const (
	walkxcodeIcons = "https://cdn.jsdelivr.net/gh/homarr-labs/dashboard-icons@master/tree.json"
	selfhstIcons   = "https://raw.githubusercontent.com/selfhst/icons/refs/heads/main/index.json"
)

type provider struct{}

func (p provider) HasIcon(u *icons.URL) bool {
	return HasIcon(u)
}

func init() {
	icons.SetProvider(provider{})
}

func InitCache() {
	m := make(IconMap)
	err := serialization.LoadJSONIfExist(common.IconListCachePath, &m)
	if err != nil {
		// backward compatible
		oldFormat := struct {
			Icons      IconMap
			LastUpdate time.Time
		}{}
		err = serialization.LoadJSONIfExist(common.IconListCachePath, &oldFormat)
		if err != nil {
			log.Error().Err(err).Msg("failed to load icons")
		} else {
			m = oldFormat.Icons
			// store it to disk immediately
			_ = serialization.SaveJSON(common.IconListCachePath, &m, 0o644)
		}
	} else if len(m) > 0 {
		log.Info().
			Int("icons", len(m)).
			Msg("icons loaded")
	} else {
		if err := updateIcons(m); err != nil {
			log.Error().Err(err).Msg("failed to update icons")
		}
	}

	iconsCache.Store(m)

	task.OnProgramExit("save_icons_cache", func() {
		icons := iconsCache.Load()
		_ = serialization.SaveJSON(common.IconListCachePath, &icons, 0o644)
	})

	go backgroundUpdateIcons()
}

func backgroundUpdateIcons() {
	ticker := time.NewTicker(updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			log.Info().Msg("updating icon data")
			newCache := make(IconMap, len(iconsCache.Load()))
			if err := updateIcons(newCache); err != nil {
				log.Error().Err(err).Msg("failed to update icons")
			} else {
				// swap old cache with new cache
				iconsCache.Store(newCache)
				// save it to disk
				err := serialization.SaveJSON(common.IconListCachePath, &newCache, 0o644)
				if err != nil {
					log.Warn().Err(err).Msg("failed to save icons")
				}
				log.Info().Int("icons", len(newCache)).Msg("icons list updated")
			}
		case <-task.RootContext().Done():
			return
		}
	}
}

func TestClearIconsCache() {
	clear(iconsCache.Load())
}

func ListAvailableIcons() IconMap {
	return iconsCache.Load()
}

func SearchIcons(keyword string, limit int) []*IconMetaSearch {
	if keyword == "" {
		return []*IconMetaSearch{}
	}

	if limit == 0 {
		limit = 10
	}

	searchLimit := min(limit*5, 50)

	results := make([]*IconMetaSearch, 0, searchLimit)

	sortByRank := func(a, b *IconMetaSearch) int {
		return a.rank - b.rank
	}

	var rank int
	icons := ListAvailableIcons()
	for k, icon := range icons {
		if strutils.ContainsFold(string(k), keyword) || strutils.ContainsFold(icon.DisplayName, keyword) {
			rank = 0
		} else {
			rank = fuzzy.RankMatchFold(keyword, string(k))
			if rank == -1 || rank > 3 {
				continue
			}
		}

		source, ref := k.SourceRef()
		ranked := &IconMetaSearch{
			Source: source,
			Ref:    ref,
			Meta:   icon,
			rank:   rank,
		}
		// Sorted insert based on rank (lower rank = better match)
		insertPos, _ := slices.BinarySearchFunc(results, ranked, sortByRank)
		results = slices.Insert(results, insertPos, ranked)
		if len(results) == searchLimit {
			break
		}
	}

	// Extract results and limit to the requested count
	return results[:min(len(results), limit)]
}

func HasIcon(icon *icons.URL) bool {
	if icon.Extra == nil {
		return false
	}
	if common.IsTest {
		return true
	}
	meta, ok := ListAvailableIcons()[icon.Extra.Key]
	if !ok {
		return false
	}
	switch icon.Extra.FileType {
	case "png":
		return meta.PNG && (!icon.Extra.IsLight || meta.Light) && (!icon.Extra.IsDark || meta.Dark)
	case "svg":
		return meta.SVG && (!icon.Extra.IsLight || meta.Light) && (!icon.Extra.IsDark || meta.Dark)
	case "webp":
		return meta.WebP && (!icon.Extra.IsLight || meta.Light) && (!icon.Extra.IsDark || meta.Dark)
	default:
		return false
	}
}

type HomepageMeta struct {
	DisplayName string
	Tag         string
}

func GetMetadata(ref string) (HomepageMeta, bool) {
	meta, ok := ListAvailableIcons()[icons.NewKey(icons.SourceSelfhSt, ref)]
	// these info is not available in walkxcode
	// if !ok {
	// 	meta, ok = iconsCache.Icons[icons.NewIconKey(icons.IconSourceWalkXCode, ref)]
	// }
	if !ok {
		return HomepageMeta{}, false
	}
	return HomepageMeta{
		DisplayName: meta.DisplayName,
		Tag:         meta.Tag,
	}, true
}

func updateIcons(m IconMap) error {
	if err := UpdateWalkxCodeIcons(m); err != nil {
		return err
	}
	return UpdateSelfhstIcons(m)
}

var httpGet = httpGetImpl

func MockHTTPGet(body []byte) {
	httpGet = func(_ string) ([]byte, func([]byte), error) {
		return body, func([]byte) {}, nil
	}
}

func httpGetImpl(url string) ([]byte, func([]byte), error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	return httputils.ReadAllBody(resp)
}

/*
format:

	{
		"png": [
			"*.png",
		],
		"svg": [
			"*.svg",
		],
		"webp": [
			"*.webp",
		]
	}
*/
func UpdateWalkxCodeIcons(m IconMap) error {
	body, release, err := httpGet(walkxcodeIcons)
	if err != nil {
		return err
	}

	data := make(map[string][]string)
	err = json.Unmarshal(body, &data)
	release(body)
	if err != nil {
		return err
	}

	for fileType, files := range data {
		var setExt func(icon *icons.Meta)
		switch fileType {
		case "png":
			setExt = func(icon *icons.Meta) { icon.PNG = true }
		case "svg":
			setExt = func(icon *icons.Meta) { icon.SVG = true }
		case "webp":
			setExt = func(icon *icons.Meta) { icon.WebP = true }
		}
		for _, f := range files {
			f = strings.TrimSuffix(f, "."+fileType)
			isLight := strings.HasSuffix(f, "-light")
			if isLight {
				f = strings.TrimSuffix(f, "-light")
			}
			isDark := strings.HasSuffix(f, "-dark")
			if isDark {
				f = strings.TrimSuffix(f, "-dark")
			}
			key := icons.NewKey(icons.SourceWalkXCode, f)
			icon, ok := m[key]
			if !ok {
				icon = new(icons.Meta)
				m[key] = icon
			}
			setExt(icon)
			if isLight {
				icon.Light = true
			}
			if isDark {
				icon.Dark = true
			}
		}
	}
	return nil
}

/*
format:

	{
			"Name": "2FAuth",
			"Reference": "2fauth",
			"SVG": "Yes",
			"PNG": "Yes",
			"WebP": "Yes",
			"Light": "Yes",
			"Dark": "Yes",
			"Tag": "",
			"Category": "Self-Hosted",
			"CreatedAt": "2024-08-16 00:27:23+00:00"
	}
*/

func UpdateSelfhstIcons(m IconMap) error {
	type SelfhStIcon struct {
		Name      string
		Reference string
		SVG       string
		PNG       string
		WebP      string
		Light     string
		Dark      string
		Tags      string
	}

	body, release, err := httpGet(selfhstIcons)
	if err != nil {
		return err
	}

	data := make([]SelfhStIcon, 0)
	err = json.Unmarshal(body, &data) //nolint:musttag
	release(body)
	if err != nil {
		return err
	}

	for _, item := range data {
		var tag string
		if item.Tags != "" {
			tag, _, _ = strings.Cut(item.Tags, ",")
			tag = strings.TrimSpace(tag)
		}
		icon := &icons.Meta{
			DisplayName: item.Name,
			Tag:         intern.Make(tag).Value(),
			SVG:         item.SVG == "Yes",
			PNG:         item.PNG == "Yes",
			WebP:        item.WebP == "Yes",
			Light:       item.Light == "Yes",
			Dark:        item.Dark == "Yes",
		}
		key := icons.NewKey(icons.SourceSelfhSt, item.Reference)
		m[key] = icon
	}
	return nil
}
