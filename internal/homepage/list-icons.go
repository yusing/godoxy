package homepage

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/utils"
)

type GitHubContents struct { //! keep this, may reuse in future
	Type string `json:"type"`
	Path string `json:"path"`
	Name string `json:"name"`
	Sha  string `json:"sha"`
	Size int    `json:"size"`
}

type (
	IconsMap map[string]map[string]struct{}
	IconList []string
	Cache    struct {
		WalkxCode, Selfhst IconsMap
		DisplayNames       ReferenceDisplayNameMap
		IconList           IconList // combined into a single list
	}
	ReferenceDisplayNameMap map[string]string
)

func (icons *Cache) needUpdate() bool {
	return len(icons.WalkxCode) == 0 || len(icons.Selfhst) == 0 || len(icons.IconList) == 0 || len(icons.DisplayNames) == 0
}

const updateInterval = 2 * time.Hour

var (
	iconsCache   *Cache
	iconsCahceMu sync.RWMutex
	lastUpdate   time.Time
)

const (
	walkxcodeIcons = "https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons@master/tree.json"
	selfhstIcons   = "https://cdn.selfh.st/directory/icons.json"
)

func InitIconListCache() {
	iconsCahceMu.Lock()
	defer iconsCahceMu.Unlock()

	iconsCache = &Cache{
		WalkxCode:    make(IconsMap),
		Selfhst:      make(IconsMap),
		DisplayNames: make(ReferenceDisplayNameMap),
		IconList:     []string{},
	}
	// err := utils.LoadJSONIfExist(common.IconListCachePath, iconsCache)
	// if err != nil {
	// 	logging.Error().Err(err).Msg("failed to load icon list cache config")
	// } else if len(iconsCache.IconList) > 0 {
	// 	logging.Info().
	// 		Int("icons", len(iconsCache.IconList)).
	// 		Int("display_names", len(iconsCache.DisplayNames)).
	// 		Msg("icon list cache loaded")
	// }
}

func ListAvailableIcons() (*Cache, error) {
	iconsCahceMu.RLock()
	if time.Since(lastUpdate) < updateInterval {
		if !iconsCache.needUpdate() {
			iconsCahceMu.RUnlock()
			return iconsCache, nil
		}
	}
	iconsCahceMu.RUnlock()

	iconsCahceMu.Lock()
	defer iconsCahceMu.Unlock()

	logging.Info().Msg("updating icon data")
	icons, err := fetchIconData()
	if err != nil {
		return nil, err
	}
	logging.Info().
		Int("icons", len(icons.IconList)).
		Int("display_names", len(icons.DisplayNames)).
		Msg("icons list updated")

	iconsCache = icons
	lastUpdate = time.Now()

	err = utils.SaveJSON(common.IconListCachePath, iconsCache, 0o644)
	if err != nil {
		logging.Warn().Err(err).Msg("failed to save icon list cache")
	}
	return icons, nil
}

func SearchIcons(keyword string, limit int) ([]string, error) {
	icons, err := ListAvailableIcons()
	if err != nil {
		return nil, err
	}
	if keyword == "" {
		return utils.Slice(icons.IconList, limit), nil
	}
	return utils.Slice(fuzzy.Find(keyword, icons.IconList), limit), nil
}

func HasWalkxCodeIcon(name string, filetype string) bool {
	icons, err := ListAvailableIcons()
	if err != nil {
		logging.Error().Err(err).Msg("failed to list icons")
		return false
	}
	if _, ok := icons.WalkxCode[filetype]; !ok {
		return false
	}
	_, ok := icons.WalkxCode[filetype][name+"."+filetype]
	return ok
}

func HasSelfhstIcon(name string, filetype string) bool {
	icons, err := ListAvailableIcons()
	if err != nil {
		logging.Error().Err(err).Msg("failed to list icons")
		return false
	}
	if _, ok := icons.Selfhst[filetype]; !ok {
		return false
	}
	_, ok := icons.Selfhst[filetype][name+"."+filetype]
	return ok
}

func GetDisplayName(reference string) (string, bool) {
	icons, err := ListAvailableIcons()
	if err != nil {
		logging.Error().Err(err).Msg("failed to list icons")
		return "", false
	}
	displayName, ok := icons.DisplayNames[reference]
	return displayName, ok
}

func fetchIconData() (*Cache, error) {
	walkxCodeIconMap, walkxCodeIconList, err := fetchWalkxCodeIcons()
	if err != nil {
		return nil, err
	}

	n := 0
	for _, items := range walkxCodeIconMap {
		n += len(items)
	}

	selfhstIconMap, selfhstIconList, referenceToNames, err := fetchSelfhstIcons()
	if err != nil {
		return nil, err
	}

	return &Cache{
		WalkxCode:    walkxCodeIconMap,
		Selfhst:      selfhstIconMap,
		DisplayNames: referenceToNames,
		IconList:     append(walkxCodeIconList, selfhstIconList...),
	}, nil
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
func fetchWalkxCodeIcons() (IconsMap, IconList, error) {
	req, err := http.NewRequest(http.MethodGet, walkxcodeIcons, nil)
	if err != nil {
		return nil, nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	data := make(map[string][]string)
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, nil, err
	}
	icons := make(IconsMap, len(data))
	iconList := make(IconList, 0, 2000)
	for fileType, files := range data {
		icons[fileType] = make(map[string]struct{}, len(files))
		for _, icon := range files {
			icons[fileType][icon] = struct{}{}
			iconList = append(iconList, "@walkxcode/"+icon)
		}
	}
	return icons, iconList, nil
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
			"Category": "Self-Hosted",
			"CreatedAt": "2024-08-16 00:27:23+00:00"
	}
*/
func fetchSelfhstIcons() (IconsMap, IconList, ReferenceDisplayNameMap, error) {
	type SelfhStIcon struct {
		Name      string `json:"Name"`
		Reference string `json:"Reference"`
		SVG       string `json:"SVG"`
		PNG       string `json:"PNG"`
		WebP      string `json:"WebP"`
		// Light          string
		// Category       string
		// CreatedAt      string
	}

	req, err := http.NewRequest(http.MethodGet, selfhstIcons, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, nil, err
	}

	data := make([]SelfhStIcon, 0, 2000)
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, nil, nil, err
	}

	iconList := make(IconList, 0, len(data)*3)
	icons := make(IconsMap)
	icons["svg"] = make(map[string]struct{}, len(data))
	icons["png"] = make(map[string]struct{}, len(data))
	icons["webp"] = make(map[string]struct{}, len(data))

	referenceToNames := make(ReferenceDisplayNameMap, len(data))

	for _, item := range data {
		if item.SVG == "Yes" {
			icons["svg"][item.Reference+".svg"] = struct{}{}
			iconList = append(iconList, "@selfhst/"+item.Reference+".svg")
		}
		if item.PNG == "Yes" {
			icons["png"][item.Reference+".png"] = struct{}{}
			iconList = append(iconList, "@selfhst/"+item.Reference+".png")
		}
		if item.WebP == "Yes" {
			icons["webp"][item.Reference+".webp"] = struct{}{}
			iconList = append(iconList, "@selfhst/"+item.Reference+".webp")
		}
		referenceToNames[item.Reference] = item.Name
	}

	return icons, iconList, referenceToNames, nil
}
