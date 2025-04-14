package migrations

import (
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/pkg"
)

func RunMigrations() error {
	if currentVersion.IsEqual(lastVersion) {
		return nil
	}
	logging.Info().Msg("running migrations...")
	errs := gperr.NewBuilder("migration error")
	for _, m := range migrations {
		if !currentVersion.IsOlderThan(m.Since) {
			continue
		}
		if err := m.Run(); err != nil {
			errs.Add(gperr.PrependSubject(m.Name, err))
		}
	}
	return errs.Error()
}

var currentVersion = pkg.GetVersion()
var lastVersion = pkg.GetLastVersion()
var migrations = []migration{
	{"move json data", m001_move_json_data, pkg.Ver(0, 11, 0)},
}
