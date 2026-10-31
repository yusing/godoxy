package provider

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func fileReloadFixture(t *testing.T, definition string) (*Provider, *task.Task, *entrypoint.Entrypoint, string) {
	t.Helper()
	oldHTTPAddr, oldHTTPSAddr := common.ProxyHTTPAddr, common.ProxyHTTPSAddr
	common.ProxyHTTPAddr, common.ProxyHTTPSAddr = "127.0.0.1:0", ""
	t.Cleanup(func() { common.ProxyHTTPAddr, common.ProxyHTTPSAddr = oldHTTPAddr, oldHTTPSAddr })

	routeFile := filepath.Join(t.TempDir(), "routes.yml")
	require.NoError(t, os.WriteFile(routeFile, []byte(definition), 0o600))

	root := task.RootTask("unchanged-route-reload", false)
	t.Cleanup(func() { root.FinishAndWait("test finished") })
	ep := entrypoint.NewEntrypoint(root, nil)
	entrypoint.SetCtx(root, ep)
	p := newProvider(routing.ProviderTypeFile)
	p.ProviderImpl = &FileProvider{fileName: "routes.yml", path: routeFile}
	p.watcher = noopWatcher{}

	require.NoError(t, p.LoadRoutes(root.Context()))
	activation := p.Activate(root)
	require.NoError(t, activation.InfrastructureError)
	require.Empty(t, activation.FailedRoutes)
	return p, root, ep, routeFile
}

func reloadFile(p *Provider, root *task.Task) {
	p.newEventHandler().Handle(root, []watcher.Event{{Type: watcherEvents.EventTypeFile, Action: watcherEvents.ActionFileWritten, ActorName: "routes.yml"}})
}

// Editing one route in a file must not restart the other routes in it.
func TestFileReloadRestartsOnlyChangedRoutes(t *testing.T) {
	p, root, _, routeFile := fileReloadFixture(t, unchangedRoutesYAML)
	plexBefore, ok := p.GetRoute("plex")
	require.True(t, ok)
	nasBefore, ok := p.GetRoute("nas")
	require.True(t, ok)

	// rewrite with only nas changed
	edited := unchangedRoutesYAML[:len(unchangedRoutesYAML)-len("    disabled: true\n")] + "    disabled: true\n  homepage:\n    name: NAS\n"
	require.NoError(t, os.WriteFile(routeFile, []byte(edited), 0o600))
	reloadFile(p, root)

	plexAfter, ok := p.GetRoute("plex")
	require.True(t, ok)
	nasAfter, ok := p.GetRoute("nas")
	require.True(t, ok)
	require.Same(t, plexBefore, plexAfter, "unchanged route must keep running")
	require.Nil(t, plexBefore.Task().FinishCause(), "unchanged route must not be stopped")
	require.NotSame(t, nasBefore, nasAfter, "changed route must be replaced")
	require.Equal(t, "NAS", nasAfter.HomepageItem().Name)

	// re-saving identical content restarts nothing
	reloadFile(p, root)
	plexAgain, _ := p.GetRoute("plex")
	nasAgain, _ := p.GetRoute("nas")
	require.Same(t, plexAfter, plexAgain)
	require.Same(t, nasAfter, nasAgain)

	// a force reload still restarts everything
	p.newEventHandler().Handle(root, forceReloadEvents())
	plexForced, _ := p.GetRoute("plex")
	require.NotSame(t, plexAfter, plexForced)
}

func TestFileReloadKeepsCanonicalAndDefaultRoutes(t *testing.T) {
	for name, definition := range map[string]string{
		"normalized alias": "' App ':\n  host: 192.0.2.10\n  port: 8080\n  healthcheck:\n    disabled: true\n",
		"default route":    "portainer: null\n",
	} {
		t.Run(name, func(t *testing.T) {
			p, root, ep, _ := fileReloadFixture(t, definition)
			require.Equal(t, 1, p.NumRoutes())
			for alias, before := range p.lockCloneRoutes() {
				require.Equal(t, before.Alias, alias)
				require.NotEmpty(t, before.SourceFingerprint())
				reloadFile(p, root)
				after, ok := p.lockGetRoute(alias)
				require.True(t, ok)
				require.Same(t, before, after)
				require.Nil(t, before.Task().FinishCause())
				active, ok := ep.HTTPRoutes().Get(alias)
				require.True(t, ok)
				require.Same(t, before.Impl(), active)
			}
			require.Equal(t, 1, p.NumRoutes())
		})
	}
}

func TestFileProviderRejectsCanonicalAliasCollisionsOnFirstLoad(t *testing.T) {
	definition := "app:\n  host: 192.0.2.10\n  port: 8080\nApp:\n  host: 192.0.2.11\n  port: 8081\n' app ':\n  host: 192.0.2.12\n  port: 8082\n"
	file := filepath.Join(t.TempDir(), "routes.yml")
	require.NoError(t, os.WriteFile(file, []byte(definition), 0o600))
	root := task.RootTask(t.Name(), false)
	t.Cleanup(func() { root.FinishAndWait("test finished") })
	p := newProvider(routing.ProviderTypeFile)
	p.ProviderImpl = &FileProvider{fileName: "routes.yml", path: file}
	p.watcher = noopWatcher{}

	require.ErrorContains(t, p.LoadRoutes(root.Context()), "normalize to the same alias")
	require.Zero(t, p.NumRoutes(), "no colliding definition may become a candidate")
	activation := p.Activate(root)
	require.Equal(t, 3, activation.DesiredRoutes)
	require.Zero(t, activation.AttemptedRoutes)
	require.Zero(t, activation.ActiveRoutes)
	require.Len(t, activation.FailedRoutes, 3)
	aliases := make([]string, 0, len(activation.FailedRoutes))
	for _, issue := range activation.FailedRoutes {
		aliases = append(aliases, issue.Route)
		require.ErrorContains(t, issue.Err, "normalize to the same alias")
	}
	require.ElementsMatch(t, []string{"app", "App", " app "}, aliases)
}

func TestFileReloadRetainsAmbiguousAliasAndUpdatesValidSibling(t *testing.T) {
	p, root, ep, file := fileReloadFixture(t, unchangedRoutesYAML)
	before, ok := p.lockGetRoute("plex")
	require.True(t, ok)
	nasBefore, ok := p.lockGetRoute("nas")
	require.True(t, ok)
	candidate := strings.Replace(unchangedRoutesYAML, "192.0.2.31", "192.0.2.32", 1) + "Plex:\n  host: 192.0.2.99\n  port: 8080\n"
	require.NoError(t, os.WriteFile(file, []byte(candidate), 0o600))
	reloadFile(p, root)

	after, ok := p.lockGetRoute("plex")
	require.True(t, ok)
	require.Same(t, before, after, "ambiguous replacement must retain the running instance")
	require.Nil(t, before.Task().FinishCause())
	active, ok := ep.HTTPRoutes().Get("plex")
	require.True(t, ok)
	require.Same(t, before.Impl(), active)
	nasAfter, ok := p.lockGetRoute("nas")
	require.True(t, ok)
	require.NotSame(t, nasBefore, nasAfter)
	require.Equal(t, "192.0.2.32", nasAfter.Host)
	activation := p.ActivationSnapshot()
	require.Equal(t, 3, activation.DesiredRoutes)
	require.Equal(t, 1, activation.AttemptedRoutes)
	require.Equal(t, 2, activation.ActiveRoutes)
	require.Len(t, activation.FailedRoutes, 2)
}

func TestFileReloadReaddsExcludedRoute(t *testing.T) {
	definition := "app-old:\n  host: 192.0.2.10\n  port: 8080\n  healthcheck:\n    disabled: true\n"
	p, root, ep, file := fileReloadFixture(t, definition)
	before, ok := ep.ExcludedRoutes().Get("routes:app-old")
	require.True(t, ok)
	require.NoError(t, os.WriteFile(file, []byte("{}\n"), 0o600))
	reloadFile(p, root)
	_, ok = ep.ExcludedRoutes().Get("routes:app-old")
	require.False(t, ok)
	require.Zero(t, p.NumRoutes())
	require.NoError(t, os.WriteFile(file, []byte(definition), 0o600))
	reloadFile(p, root)
	after, ok := ep.ExcludedRoutes().Get("routes:app-old")
	require.True(t, ok, "route commands must find the re-added excluded route")
	require.NotSame(t, before, after)
	require.Equal(t, 1, p.NumRoutes())
}

func TestFileReloadReadsChangedRuleFile(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("custom.rules", []byte("default { error 403 old }\n"), 0o600))
	definition := "app:\n  host: 192.0.2.10\n  port: 8080\n  healthcheck:\n    disabled: true\n  rule_file: custom.rules\n"
	p, root, ep, _ := fileReloadFixture(t, definition)
	response := func() int {
		r, ok := ep.HTTPRoutes().Get("app")
		require.True(t, ok)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://app/", nil))
		return w.Code
	}
	require.Equal(t, http.StatusForbidden, response())
	require.NoError(t, os.WriteFile("custom.rules", []byte("default { error 401 new }\n"), 0o600))
	reloadFile(p, root)
	require.Equal(t, http.StatusUnauthorized, response())
}
