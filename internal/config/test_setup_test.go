package config

import (
	"os"
	"testing"

	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/routevalidate"
)

func TestMain(m *testing.M) {
	route.InitBuilder(routevalidate.Validate)
	os.Exit(m.Run())
}
