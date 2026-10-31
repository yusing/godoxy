package provider

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/entrypoint"
	"github.com/yusing/godoxy/internal/routing"
	"github.com/yusing/godoxy/internal/watcher"
	watcherEvents "github.com/yusing/godoxy/internal/watcher/events"
	"github.com/yusing/goutils/task"
)

const unchangedRoutesYAML = `plex:
  scheme: http
  host: 192.0.2.30
  port: 32400
  healthcheck:
    disabled: true
nas:
  scheme: http
  host: 192.0.2.31
  port: 5000
  healthcheck:
    disabled: true
`

// Editing one route in a file must not restart the other routes in it.
func TestFileReloadRestartsOnlyChangedRoutes(t *testing.T) {
	oldHTTPAddr, oldHTTPSAddr := common.ProxyHTTPAddr, common.ProxyHTTPSAddr
	common.ProxyHTTPAddr, common.ProxyHTTPSAddr = "127.0.0.1:0", ""
	t.Cleanup(func() { common.ProxyHTTPAddr, common.ProxyHTTPSAddr = oldHTTPAddr, oldHTTPSAddr })

	routeFile := filepath.Join(t.TempDir(), "routes.yml")
	require.NoError(t, os.WriteFile(routeFile, []byte(unchangedRoutesYAML), 0o600))

	root := task.RootTask("unchanged-route-reload", false)
	t.Cleanup(func() { root.FinishAndWait("test finished") })
	ep := entrypoint.NewEntrypoint(root, nil)
	entrypoint.SetCtx(root, ep)
	p := newProvider(routing.ProviderTypeFile)
	p.ProviderImpl = &FileProvider{fileName: "routes.yml", path: routeFile}
	p.watcher = noopWatcher{}

	require.NoError(t, p.LoadRoutes(root.Context()))
	require.NoError(t, p.Activate(root).InfrastructureError)

	plexBefore, ok := p.GetRoute("plex")
	require.True(t, ok)
	nasBefore, ok := p.GetRoute("nas")
	require.True(t, ok)

	written := []watcher.Event{{Type: watcherEvents.EventTypeFile, Action: watcherEvents.ActionFileWritten, ActorName: "routes.yml"}}

	// rewrite with only nas changed
	edited := unchangedRoutesYAML[:len(unchangedRoutesYAML)-len("    disabled: true\n")] + "    disabled: true\n  homepage:\n    name: NAS\n"
	require.NoError(t, os.WriteFile(routeFile, []byte(edited), 0o600))
	p.newEventHandler().Handle(root, written)

	plexAfter, ok := p.GetRoute("plex")
	require.True(t, ok)
	nasAfter, ok := p.GetRoute("nas")
	require.True(t, ok)
	require.Same(t, plexBefore, plexAfter, "unchanged route must keep running")
	require.Nil(t, plexBefore.Task().FinishCause(), "unchanged route must not be stopped")
	require.NotSame(t, nasBefore, nasAfter, "changed route must be replaced")
	require.Equal(t, "NAS", nasAfter.HomepageItem().Name)

	// re-saving identical content restarts nothing
	p.newEventHandler().Handle(root, written)
	plexAgain, _ := p.GetRoute("plex")
	nasAgain, _ := p.GetRoute("nas")
	require.Same(t, plexAfter, plexAgain)
	require.Same(t, nasAfter, nasAgain)

	// a force reload still restarts everything
	p.newEventHandler().Handle(root, forceReloadEvents())
	plexForced, _ := p.GetRoute("plex")
	require.NotSame(t, plexAfter, plexForced)
}
