package homepage

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/serialization"
	httputils "github.com/yusing/goutils/http"
	strutils "github.com/yusing/goutils/strings"
	"github.com/yusing/goutils/synk"
	"github.com/yusing/goutils/task"
)

type (
	IconKey  string
	IconMap  map[IconKey]*IconMeta
	IconList []string
	IconMeta struct {
		SVG         bool   `json:"SVG"`
		PNG         bool   `json:"PNG"`
		WebP        bool   `json:"WebP"`
		Light       bool   `json:"Light"`
		Dark        bool   `json:"Dark"`
		DisplayName string `json:"-"`
		Tag         string `json:"-"`
	}
	IconMetaSearch struct {
		*IconMeta

		Source IconSource `json:"Source"`
		Ref    string     `json:"Ref"`

		rank int
	}
)

func (icon *IconMeta) Filenames(ref string) []string {
	filenames := make([]string, 0)
	if icon.SVG {
		filenames = append(filenames, ref+".svg")
		if icon.Light {
			filenames = append(filenames, ref+"-light.svg")
		}
		if icon.Dark {
			filenames = append(filenames, ref+"-dark.svg")
		}
	}
	if icon.PNG {
		filenames = append(filenames, ref+".png")
		if icon.Light {
			filenames = append(filenames, ref+"-light.png")
		}
		if icon.Dark {
			filenames = append(filenames, ref+"-dark.png")
		}
	}
	if icon.WebP {
		filenames = append(filenames, ref+".webp")
		if icon.Light {
			filenames = append(filenames, ref+"-light.webp")
		}
		if icon.Dark {
			filenames = append(filenames, ref+"-dark.webp")
		}
	}
	return filenames
}

const updateInterval = 2 * time.Hour

var iconsCache synk.Value[IconMap]

const (
	walkxcodeIcons = "https://cdn.jsdelivr.net/gh/homarr-labs/dashboard-icons@master/tree.json"
	selfhstIcons   = "https://raw.githubusercontent.com/selfhst/icons/refs/heads/main/index.json"
)

func NewIconKey(source IconSource, reference string) IconKey {
	return IconKey(fmt.Sprintf("%s/%s", source, reference))
}

func (k IconKey) SourceRef() (IconSource, string) {
	source, ref, _ := strings.Cut(string(k), "/")
	return IconSource(source), ref
}

func InitIconListCache() {
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
			Source:   source,
			Ref:      ref,
			IconMeta: icon,
			rank:     rank,
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

func HasIcon(icon *IconURL) bool {
	if icon.Extra == nil {
		return false
	}
	if common.IsTest {
		return true
	}
	key := NewIconKey(icon.IconSource, icon.Extra.Ref)
	meta, ok := ListAvailableIcons()[key]
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

func GetHomepageMeta(ref string) (HomepageMeta, bool) {
	meta, ok := ListAvailableIcons()[NewIconKey(IconSourceSelfhSt, ref)]
	// these info is not available in walkxcode
	// if !ok {
	// 	meta, ok = iconsCache.Icons[NewIconKey(IconSourceWalkXCode, ref)]
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
	err = sonic.Unmarshal(body, &data)
	release(body)
	if err != nil {
		return err
	}

	for fileType, files := range data {
		var setExt func(icon *IconMeta)
		switch fileType {
		case "png":
			setExt = func(icon *IconMeta) { icon.PNG = true }
		case "svg":
			setExt = func(icon *IconMeta) { icon.SVG = true }
		case "webp":
			setExt = func(icon *IconMeta) { icon.WebP = true }
		}
		for _, f := range files {
			f = strings.TrimSuffix(f, "."+fileType)
			isLight := strings.HasSuffix(f, "-light")
			if isLight {
				f = strings.TrimSuffix(f, "-light")
			}
			key := NewIconKey(IconSourceWalkXCode, f)
			icon, ok := m[key]
			if !ok {
				icon = new(IconMeta)
				m[key] = icon
			}
			setExt(icon)
			if isLight {
				icon.Light = true
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
	err = sonic.Unmarshal(body, &data) //nolint:musttag
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
		icon := &IconMeta{
			DisplayName: item.Name,
			Tag:         tag,
			SVG:         item.SVG == "Yes",
			PNG:         item.PNG == "Yes",
			WebP:        item.WebP == "Yes",
			Light:       item.Light == "Yes",
			Dark:        item.Dark == "Yes",
		}
		key := NewIconKey(IconSourceSelfhSt, item.Reference)
		m[key] = icon
	}
	return nil
}
