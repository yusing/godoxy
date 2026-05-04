package iconlist

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/homepage/icons"
	"github.com/yusing/godoxy/internal/serialization"
	gperr "github.com/yusing/goutils/errs"
	httputils "github.com/yusing/goutils/http"
	"github.com/yusing/goutils/intern"
	strutils "github.com/yusing/goutils/strings"
	"github.com/yusing/goutils/task"
)

type (
	IconMap  = *xsync.Map[icons.Key, *icons.Meta]
	IconList []string

	IconMetaSearch struct {
		*icons.Meta

		Source icons.Source `json:"Source"`
		Ref    string       `json:"Ref"`

		rank int
	} // @name IconMetaSearch
)

const updateInterval = 2 * time.Hour

var iconsCache atomic.Pointer[xsync.Map[icons.Key, *icons.Meta]]

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
	m := NewIconMap()
	err := serialization.LoadFileIfExist(common.IconListCachePath, &m, strutils.UnmarshalJSON)
	switch {
	case err != nil:
		// backward compatible
		oldFormat := struct {
			Icons      IconMap
			LastUpdate time.Time
		}{}
		err = serialization.LoadFileIfExist(common.IconListCachePath, &oldFormat, strutils.UnmarshalJSON)
		if err != nil {
			log.Error().Err(err).Msg("failed to load icons")
		} else {
			m = oldFormat.Icons
			// store it to disk immediately
			_ = serialization.SaveFile(common.IconListCachePath, &m, 0o644, strutils.MarshalJSON)
		}
	case m.Size() > 0:
		log.Info().
			Int("icons", m.Size()).
			Msg("icons loaded")
	default:
		if err := updateIcons(m); err != nil {
			log.Error().Err(err).Msg("failed to update icons")
		}
	}

	iconsCache.Store(m)

	task.OnProgramExit("save_icons_cache", func() {
		icons := iconsCache.Load()
		_ = serialization.SaveFile(common.IconListCachePath, &icons, 0o644, strutils.MarshalJSON)
	})

	go backgroundUpdateIcons()
}

func NewIconMap(options ...func(*xsync.MapConfig)) *xsync.Map[icons.Key, *icons.Meta] {
	return xsync.NewMap[icons.Key, *icons.Meta](options...)
}

func backgroundUpdateIcons() {
	ticker := time.NewTicker(updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			log.Info().Msg("updating icon data")
			newCache := NewIconMap(xsync.WithPresize(iconsCache.Load().Size()))
			if err := updateIcons(newCache); err != nil {
				log.Error().Err(err).Msg("failed to update icons")
			} else {
				// swap old cache with new cache
				iconsCache.Store(newCache)
				// save it to disk
				err := serialization.SaveFile(common.IconListCachePath, &newCache, 0o644, strutils.MarshalJSON)
				if err != nil {
					log.Warn().Err(err).Msg("failed to save icons")
				}
				log.Info().Int("icons", newCache.Size()).Msg("icons list updated")
			}
		case <-task.RootContext().Done():
			return
		}
	}
}

func TestClearIconsCache() {
	if cache := iconsCache.Load(); cache != nil {
		cache.Clear()
	}
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

	dashedKeyword := strings.ReplaceAll(keyword, " ", "-")
	whitespacedKeyword := strings.ReplaceAll(keyword, "-", " ")

	icons := ListAvailableIcons()
	for k, icon := range icons.Range {
		source, ref := k.SourceRef()

		var rank int
		switch {
		case strings.EqualFold(ref, dashedKeyword):
			// exact match: best rank, use source as tiebreaker (lower index = higher priority)
			rank = 0
		case strutils.HasPrefixFold(ref, dashedKeyword):
			// prefix match: rank by how much extra the name has (shorter = better)
			rank = 100 + len(ref) - len(dashedKeyword)
		case strutils.ContainsFold(ref, dashedKeyword) || strutils.ContainsFold(icon.DisplayName, whitespacedKeyword):
			// contains match
			rank = 500 + len(ref) - len(dashedKeyword)
		default:
			rank = fuzzy.RankMatchFold(keyword, ref)
			if rank == -1 || rank > 3 {
				continue
			}
			rank += 1000
		}

		ranked := &IconMetaSearch{
			Source: source,
			Ref:    ref,
			Meta:   icon,
			rank:   rank,
		}
		results = append(results, ranked)
		if len(results) == searchLimit {
			break
		}
	}

	slices.SortStableFunc(results, sortByRank)

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
	meta, ok := ListAvailableIcons().Load(icon.Extra.Key)
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
	meta, ok := ListAvailableIcons().Load(icons.NewKey(icons.SourceSelfhSt, ref))
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errs := gperr.NewGroup("update icons")
	errs.Go(func() error {
		return UpdateWalkxCodeIcons(ctx, m)
	})
	errs.Go(func() error {
		return UpdateSelfhstIcons(ctx, m)
	})
	return errs.Wait().Error()
}

var (
	httpGet    = httpGetImpl
	httpClient = &http.Client{
		Timeout: 5 * time.Second,
	}
)

func MockHTTPGet(ctx context.Context, body []byte) {
	httpGet = func(_ context.Context, _ string) ([]byte, func([]byte), error) {
		return body, func([]byte) {}, nil
	}
}

func httpGetImpl(ctx context.Context, url string) ([]byte, func([]byte), error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	return httputils.ReadAllBody(resp)
}

/*
	UpdateWalkxCodeIcons updates the icon map with the icons from walkxcode.

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
func UpdateWalkxCodeIcons(ctx context.Context, m IconMap) error {
	body, release, err := httpGet(ctx, walkxcodeIcons)
	if err != nil {
		return err
	}

	data := make(map[string][]string)
	err = strutils.UnmarshalJSON(body, &data)
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
			icon, ok := m.Load(key)
			if !ok {
				icon = new(icons.Meta)
				m.Store(key, icon)
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

func UpdateSelfhstIcons(ctx context.Context, m IconMap) error {
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

	body, release, err := httpGet(ctx, selfhstIcons)
	if err != nil {
		return err
	}

	data := make([]SelfhStIcon, 0)
	err = strutils.UnmarshalJSON(body, &data) //nolint:musttag
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
		m.Store(key, icon)
	}
	return nil
}
