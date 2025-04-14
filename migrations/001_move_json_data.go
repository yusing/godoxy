package migrations

import (
	"errors"
	"path/filepath"

	"github.com/yusing/go-proxy/internal/common"
)

var (
	homepageJSONConfigPathOld = filepath.Join(common.ConfigDir, ".homepage.json")
	iconListCachePathOld      = filepath.Join(common.ConfigDir, ".icon_list_cache.json")
	iconCachePathOld          = filepath.Join(common.ConfigDir, ".icon_cache.json")
)

func m001_move_json_data() error {
	return errors.Join(
		mv(homepageJSONConfigPathOld, common.HomepageJSONConfigPath),
		mv(iconListCachePathOld, common.IconListCachePath),
		mv(iconCachePathOld, common.IconCachePath),
	)
}
