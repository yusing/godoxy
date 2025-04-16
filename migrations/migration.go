package migrations

import "github.com/yusing/go-proxy/pkg"

type migration struct {
	Name  string
	Run   func() error
	Since pkg.Version
}
