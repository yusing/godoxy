package migrations

import (
	"errors"
	"path/filepath"

	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/pkg"
)

var (
	HomepageJSONConfigPathOld = filepath.Join(common.ConfigDir, ".homepage.json")
	IconListCachePathOld      = filepath.Join(common.ConfigDir, ".icon_list_cache.json")
	IconCachePathOld          = filepath.Join(common.ConfigDir, ".icon_cache.json")
)

func m001_move_json_data() error {
	if version.IsOlderThan(pkg.Version{Major: 0, Minor: 11, Patch: 0}) {
		return errors.Join(
			mv(HomepageJSONConfigPathOld, common.HomepageJSONConfigPath),
			mv(IconListCachePathOld, common.IconListCachePath),
			mv(IconCachePathOld, common.IconCachePath),
		)
	}
	return nil
}
