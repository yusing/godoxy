package homepage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/serialization"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

type (
	IconKey  string
	IconMap  map[IconKey]*IconMeta
	IconList []string
	IconMeta struct {
		SVG, PNG, WebP bool
		Light, Dark    bool
		DisplayName    string
		Tag            string
	}
	IconMetaSearch struct {
		Source IconSource
		Ref    string
		SVG    bool
		PNG    bool
		WebP   bool
		Light  bool
		Dark   bool
	}
	Cache struct {
		Icons        IconMap
		LastUpdate   time.Time
		sync.RWMutex `json:"-"`
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

var iconsCache = &Cache{
	Icons: make(IconMap),
}

const (
	walkxcodeIcons = "https://cdn.jsdelivr.net/gh/homarr-labs/dashboard-icons@master/tree.json"
	selfhstIcons   = "https://selfhst.github.io/cdn/directory/icons.json"
)

func NewIconKey(source IconSource, reference string) IconKey {
	return IconKey(fmt.Sprintf("%s/%s", source, reference))
}

func (k IconKey) SourceRef() (IconSource, string) {
	parts := strings.Split(string(k), "/")
	return IconSource(parts[0]), parts[1]
}

func InitIconListCache() {
	iconsCache.Lock()
	defer iconsCache.Unlock()

	err := serialization.LoadJSONIfExist(common.IconListCachePath, iconsCache)
	if err != nil {
		log.Error().Err(err).Msg("failed to load icons")
	} else if len(iconsCache.Icons) > 0 {
		log.Info().
			Int("icons", len(iconsCache.Icons)).
			Msg("icons loaded")
	}

	if err = updateIcons(); err != nil {
		log.Error().Err(err).Msg("failed to update icons")
	}

	task.OnProgramExit("save_icons_cache", func() {
		_ = serialization.SaveJSON(common.IconListCachePath, iconsCache, 0o644)
	})
}

func ListAvailableIcons() (*Cache, error) {
	if common.IsTest {
		return iconsCache, nil
	}

	iconsCache.RLock()
	if time.Since(iconsCache.LastUpdate) < updateInterval {
		if len(iconsCache.Icons) == 0 {
			iconsCache.RUnlock()
			return iconsCache, nil
		}
	}
	iconsCache.RUnlock()

	iconsCache.Lock()
	defer iconsCache.Unlock()

	log.Info().Msg("updating icon data")
	if err := updateIcons(); err != nil {
		return nil, err
	}
	log.Info().Int("icons", len(iconsCache.Icons)).Msg("icons list updated")

	iconsCache.LastUpdate = time.Now()

	err := serialization.SaveJSON(common.IconListCachePath, iconsCache, 0o644)
	if err != nil {
		log.Warn().Err(err).Msg("failed to save icons")
	}
	return iconsCache, nil
}

func SearchIcons(keyword string, limit int) ([]IconMetaSearch, error) {
	if keyword == "" {
		return make([]IconMetaSearch, 0), nil
	}
	iconsCache.RLock()
	defer iconsCache.RUnlock()
	result := make([]IconMetaSearch, 0)
	for k, icon := range iconsCache.Icons {
		if fuzzy.MatchFold(keyword, string(k)) {
			source, ref := k.SourceRef()
			result = append(result, IconMetaSearch{
				Source: source,
				Ref:    ref,
				SVG:    icon.SVG,
				PNG:    icon.PNG,
				WebP:   icon.WebP,
				Light:  icon.Light,
				Dark:   icon.Dark,
			})
		}
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}

func HasIcon(icon *IconURL) bool {
	if icon.Extra == nil {
		return false
	}
	if common.IsTest {
		return true
	}
	iconsCache.RLock()
	defer iconsCache.RUnlock()
	key := NewIconKey(icon.IconSource, icon.Extra.Ref)
	meta, ok := iconsCache.Icons[key]
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
	iconsCache.RLock()
	defer iconsCache.RUnlock()
	meta, ok := iconsCache.Icons[NewIconKey(IconSourceSelfhSt, ref)]
	if !ok {
		return HomepageMeta{}, false
	}
	return HomepageMeta{
		DisplayName: meta.DisplayName,
		Tag:         meta.Tag,
	}, true
}

func updateIcons() error {
	clear(iconsCache.Icons)
	if err := UpdateWalkxCodeIcons(); err != nil {
		return err
	}
	return UpdateSelfhstIcons()
}

var httpGet = httpGetImpl

func MockHTTPGet(body []byte) {
	httpGet = func(_ string) ([]byte, error) {
		return body, nil
	}
}

func httpGetImpl(url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
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
func UpdateWalkxCodeIcons() error {
	body, err := httpGet(walkxcodeIcons)
	if err != nil {
		return err
	}

	data := make(map[string][]string)
	err = json.Unmarshal(body, &data)
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
			icon, ok := iconsCache.Icons[key]
			if !ok {
				icon = new(IconMeta)
				iconsCache.Icons[key] = icon
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

func UpdateSelfhstIcons() error {
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

	body, err := httpGet(selfhstIcons)
	if err != nil {
		return err
	}

	data := make([]SelfhStIcon, 0)
	err = json.Unmarshal(body, &data) //nolint:musttag
	if err != nil {
		return err
	}

	for _, item := range data {
		var tag string
		if item.Tags != "" {
			tag = strutils.CommaSeperatedList(item.Tags)[0]
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
		iconsCache.Icons[key] = icon
	}
	return nil
}
