package config

import (
	"os"
	"testing"

	"github.com/yusing/godoxy/internal/dnsproviders"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/routevalidate"
)

func TestMain(m *testing.M) {
	route.InitBuilder(routevalidate.Validate)
	// state.Init validates the autocert config, which needs the DNS providers that
	// main registers at startup.
	dnsproviders.InitProviders()
	os.Exit(m.Run())
}
