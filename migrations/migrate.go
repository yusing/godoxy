package migrations

import (
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/pkg"
)

func RunMigrations() error {
	errs := gperr.NewBuilder("migration error")
	for _, migration := range migrations {
		if err := migration(); err != nil {
			errs.Add(err)
		}
	}
	return errs.Error()
}

var version = pkg.GetVersion()
var migrations = []func() error{
	m001_move_json_data,
}
