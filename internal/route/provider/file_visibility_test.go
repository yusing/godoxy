package provider

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/agentpool"
	homepageapi "github.com/yusing/godoxy/internal/api/v1/homepage"
	routeapi "github.com/yusing/godoxy/internal/api/v1/route"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/entrypoint"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/routevalidate"
	"github.com/yusing/godoxy/internal/routing"
	"github.com/yusing/godoxy/internal/watcher"
	watcherEvents "github.com/yusing/godoxy/internal/watcher/events"
	"github.com/yusing/goutils/task"
)

var visibilityAliases = []string{"alpha", "beta.example.test", "*.example.test"}

const visibilityRoutesYAML = `alpha:
  scheme: http
  host: 192.0.2.10
  port: 8080
  healthcheck:
    disabled: true
  homepage:
    show: true
    name: Alpha
beta.example.test:
  scheme: http
  host: 192.0.2.11
  port: 8080
  healthcheck:
    disabled: true
  homepage:
    show: true
    name: Beta
"*.example.test":
  scheme: http
  host: 192.0.2.12
  port: 8080
  healthcheck:
    disabled: true
  homepage:
    show: true
    name: Wildcard
`

const partialReloadInitialRoutesYAML = `alpha:
  scheme: http
  host: 192.0.2.20
  port: 8080
  healthcheck:
    disabled: true
  homepage:
    show: true
    name: Alpha
beta:
  scheme: http
  host: 192.0.2.21
  port: 8080
  healthcheck:
    disabled: true
  homepage:
    show: true
    name: Beta
retired:
  scheme: http
  host: 192.0.2.22
  port: 8080
  healthcheck:
    disabled: true
  homepage:
    show: true
    name: Retired
`

const partialReloadCandidateRoutesYAML = `alpha:
  scheme: http
  host: 192.0.2.20
  port: 8080
  healthcheck:
    disabled: true
  homepage:
    show: true
    name: Alpha Updated
beta:
  scheme: http
  host: 192.0.2.21
  port: 8080
  healthcheck:
    disabled: true
  homepage:
    show: true
    name: Beta
gamma:
  scheme: http
  host: 192.0.2.23
  port: 8080
  healthcheck:
    disabled: true
  homepage:
    show: true
    name: Gamma
`

// TestFileProviderConfiguredRouteVisibilityAcrossReloads keeps the source file
// byte-for-byte stable while repeatedly rebuilding route inventories from it.
// Every settled snapshot checks the configured aliases through the provider,
// entrypoint, API route list, homepage, and uptime poller's source inventory.
func TestFileProviderConfiguredRouteVisibilityAcrossReloads(t *testing.T) {
	gin.SetMode(gin.TestMode)

	oldHTTPAddr, oldHTTPSAddr := common.ProxyHTTPAddr, common.ProxyHTTPSAddr
	common.ProxyHTTPAddr, common.ProxyHTTPSAddr = "127.0.0.1:0", ""
	t.Cleanup(func() { common.ProxyHTTPAddr, common.ProxyHTTPSAddr = oldHTTPAddr, oldHTTPSAddr })

	workDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(workDir, "config"), 0o755))
	routeFile := filepath.Join(workDir, "config", "routes.yml")
	require.NoError(t, os.WriteFile(routeFile, []byte(visibilityRoutesYAML), 0o600))

	root := task.RootTask("route-visibility-repro", false)
	t.Cleanup(func() { root.FinishAndWait("visibility reproduction finished") })
	ep := entrypoint.NewEntrypoint(root, nil)
	entrypoint.SetCtx(root, ep)

	// Keep the real FileProvider implementation and route lifecycle while using
	// a no-op watcher so the loop stays deterministic and does not leak a global
	// fsnotify watcher tied to t.TempDir(). File write events are passed through
	// the same provider event handler used by the watcher queue.
	p := newProvider(routing.ProviderTypeFile)
	p.ProviderImpl = &FileProvider{
		fileName: "routes.yml",
		path:     routeFile,
	}
	p.watcher = noopWatcher{}

	require.NoError(t, p.LoadRoutes(root.Context()))
	activation := p.Activate(root)
	require.NoError(t, activation.InfrastructureError)
	require.Equal(t, len(visibilityAliases), activation.ActiveRoutes)
	assertConfiguredRoutesVisible(t, root, p, ep, visibilityAliases, "initial activation")

	fileWritten := watcher.Event{
		Type:      watcherEvents.EventTypeFile,
		Action:    watcherEvents.ActionFileWritten,
		ActorName: "routes.yml",
	}
	for cycle := 1; cycle <= 32; cycle++ {
		// File events can arrive without changing the configured alias set.
		contents, err := os.ReadFile(routeFile)
		require.NoError(t, err)
		require.Equal(t, visibilityRoutesYAML, string(contents), "source config changed before reload %d", cycle)

		p.newEventHandler().Handle(root, []watcher.Event{fileWritten})
		assertConfiguredRoutesVisible(t, root, p, ep, visibilityAliases, fmt.Sprintf("file reload %02d", cycle))
	}
	t.Logf("checked %d configured aliases across initial activation and 32 unchanged-source file reloads", len(visibilityAliases))
}

func assertConfiguredRoutesVisible(t *testing.T, ctx task.Parent, p *Provider, ep *entrypoint.Entrypoint, expected []string, phase string) {
	t.Helper()
	for surface, aliases := range routeVisibilitySnapshot(t, ctx, p, ep) {
		require.ElementsMatch(t, expected, aliases, "%s omitted or duplicated configured HTTP route aliases", phase+" "+surface)
	}
}

func routeVisibilitySnapshot(t *testing.T, ctx task.Parent, p *Provider, ep *entrypoint.Entrypoint) map[string][]string {
	t.Helper()
	snapshot := map[string][]string{
		"provider inventory":   collectProviderAliases(p),
		"entrypoint inventory": collectEntrypointAliases(ep),
		"HTTP route pool":      collectHTTPAliases(ep),
	}
	routeResponse := callJSONHandler(t, ctx, "/api/v1/route/list", routeapi.Routes)
	var routes []struct {
		Alias string `json:"alias"`
	}
	require.NoError(t, json.Unmarshal(routeResponse, &routes), "/route/list response")
	routeAliases := make([]string, 0, len(routes))
	for _, route := range routes {
		routeAliases = append(routeAliases, route.Alias)
	}
	snapshot["/route/list"] = routeAliases

	homepageResponse := callJSONHandler(t, ctx, "/api/v1/homepage/items", homepageapi.Items)
	var categories []struct {
		Name  string `json:"name"`
		Items []struct {
			Alias string `json:"alias"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(homepageResponse, &categories), "/homepage/items response")
	var homepageAliases []string
	for _, category := range categories {
		if category.Name == "All" {
			for _, item := range category.Items {
				homepageAliases = append(homepageAliases, item.Alias)
			}
			break
		}
	}
	snapshot["/homepage/items All category"] = homepageAliases

	// The /metrics/uptime poller snapshots this same map from the entrypoint.
	// Checking the source here avoids depending on its background poll schedule.
	snapshot["/metrics/uptime source inventory"] = slices.Collect(maps.Keys(ep.GetHealthInfoWithoutDetail()))
	return snapshot
}

func callJSONHandler(t *testing.T, ctx task.Parent, path string, handler gin.HandlerFunc) []byte {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx.Context())
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = request
	handler(ginContext)
	require.Equal(t, http.StatusOK, recorder.Code, path+" status")
	return recorder.Body.Bytes()
}

func collectProviderAliases(p *Provider) []string {
	var aliases []string
	p.IterRoutes(func(alias string, _ routing.Route) bool {
		aliases = append(aliases, alias)
		return true
	})
	return aliases
}

func collectEntrypointAliases(ep *entrypoint.Entrypoint) []string {
	var aliases []string
	for route := range ep.IterRoutes {
		aliases = append(aliases, route.Name())
	}
	return aliases
}

func collectHTTPAliases(ep *entrypoint.Entrypoint) []string {
	var aliases []string
	for alias := range ep.HTTPRoutes().Iter {
		aliases = append(aliases, alias)
	}
	return aliases
}

func TestFileProviderRetainsConfiguredRouteWhenPartialReloadValidationFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldHTTPAddr, oldHTTPSAddr := common.ProxyHTTPAddr, common.ProxyHTTPSAddr
	common.ProxyHTTPAddr, common.ProxyHTTPSAddr = "127.0.0.1:0", ""
	t.Cleanup(func() { common.ProxyHTTPAddr, common.ProxyHTTPSAddr = oldHTTPAddr, oldHTTPSAddr })

	var rejectBeta atomic.Bool
	var betaValidationFailures atomic.Int32
	route.InitBuilder(func(ctx context.Context, configured *route.Route) (routing.Route, *agentpool.Agent, error) {
		if configured.Alias == "beta" && rejectBeta.Load() {
			betaValidationFailures.Add(1)
			return nil, nil, errors.New("transient beta validation dependency failure")
		}
		return routevalidate.Validate(ctx, configured)
	})
	t.Cleanup(func() { route.InitBuilder(routevalidate.Validate) })

	workDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(workDir, "config"), 0o755))
	routeFile := filepath.Join(workDir, "config", "routes.yml")
	require.NoError(t, os.WriteFile(routeFile, []byte(partialReloadInitialRoutesYAML), 0o600))

	root := task.RootTask("partial-route-reload-validation", false)
	t.Cleanup(func() { root.FinishAndWait("partial reload validation test finished") })
	ep := entrypoint.NewEntrypoint(root, nil)
	entrypoint.SetCtx(root, ep)
	p := newProvider(routing.ProviderTypeFile)
	p.ProviderImpl = &FileProvider{fileName: "routes.yml", path: routeFile}
	p.watcher = noopWatcher{}
	initialAliases := []string{"alpha", "beta", "retired"}
	partialAliases := []string{"alpha", "beta", "retired", "gamma"}
	cleanAliases := []string{"alpha", "beta", "gamma"}

	require.NoError(t, p.LoadRoutes(root.Context()))
	activation := p.Activate(root)
	require.NoError(t, activation.InfrastructureError)
	require.Equal(t, len(initialAliases), activation.ActiveRoutes)
	assertConfiguredRoutesVisible(t, root, p, ep, initialAliases, "before partial reload")

	// The candidate keeps beta, updates alpha, adds gamma, and removes retired.
	// During the partial failure, beta must be retained and the genuine deletion
	// of retired must wait until the provider can complete a clean load.
	require.NoError(t, os.WriteFile(routeFile, []byte(partialReloadCandidateRoutesYAML), 0o600))

	rejectBeta.Store(true)
	fileWritten := watcher.Event{
		Type:      watcherEvents.EventTypeFile,
		Action:    watcherEvents.ActionFileWritten,
		ActorName: "routes.yml",
	}
	p.newEventHandler().Handle(root, []watcher.Event{fileWritten})
	require.Equal(t, int32(1), betaValidationFailures.Load(), "beta must fail preparation once")
	require.Equal(t, 3, p.preparation.DesiredRoutes, "partial load must include all candidate aliases before validation")
	require.Len(t, p.preparation.FailedRoutes, 1, "partial load must retain its route-level validation error")
	require.Equal(t, "beta", p.preparation.FailedRoutes[0].Route)
	require.ErrorContains(t, p.preparation.FailedRoutes[0].Err, "transient beta validation dependency failure")
	contents, err := os.ReadFile(routeFile)
	require.NoError(t, err)
	require.Equal(t, partialReloadCandidateRoutesYAML, string(contents), "the source still defines beta and gamma")

	// Without the load-error guard, this assertion catches beta being removed
	// and retired's deletion being applied before the provider load is clean.
	for surface, aliases := range routeVisibilitySnapshot(t, root, p, ep) {
		assert.ElementsMatch(t, partialAliases, aliases, "%s omitted or duplicated configured HTTP route aliases", "partial reload "+surface)
	}
	assert.Equal(t, len(partialAliases), p.NumRoutes(), "all retained and newly prepared aliases must stay in the provider inventory")
	alpha, alphaOK := p.GetRoute("alpha")
	assert.True(t, alphaOK, "the healthy sibling alpha should remain active")
	if alphaOK {
		assert.Equal(t, "Alpha Updated", alpha.HomepageItem().Name, "a healthy existing sibling should receive its update")
	}
	gamma, gammaOK := p.GetRoute("gamma")
	assert.True(t, gammaOK, "the healthy new sibling gamma should be added")
	if gammaOK {
		assert.Equal(t, "Gamma", gamma.HomepageItem().Name)
	}
	_, betaOK := p.GetRoute("beta")
	assert.True(t, betaOK, "beta must remain active while its replacement fails validation")
	_, retiredOK := p.GetRoute("retired")
	assert.True(t, retiredOK, "a genuine deletion must be deferred while this provider load is partial")

	// Restore validation while leaving the candidate YAML byte-for-byte
	// unchanged. The next clean load revalidates beta and applies retired's
	// genuine deletion.
	rejectBeta.Store(false)
	p.newEventHandler().Handle(root, []watcher.Event{fileWritten})
	contents, err = os.ReadFile(routeFile)
	require.NoError(t, err)
	require.Equal(t, partialReloadCandidateRoutesYAML, string(contents), "clean recovery must read unchanged source")
	assertConfiguredRoutesVisible(t, root, p, ep, cleanAliases, "after validation dependency recovery and deletion")
	_, retiredOK = p.GetRoute("retired")
	require.False(t, retiredOK, "a clean reload should apply the genuine deletion deferred above")
	t.Log("validation recovered: beta was revalidated, alpha updated, gamma remained added, and retired was deleted on the clean reload")

	// An empty, successfully parsed route file is an intentional removal of all
	// configured routes, so it must still clear every runtime and API snapshot.
	const emptyRoutesYAML = "{}\n"
	require.NoError(t, os.WriteFile(routeFile, []byte(emptyRoutesYAML), 0o600))
	p.newEventHandler().Handle(root, []watcher.Event{fileWritten})
	contents, err = os.ReadFile(routeFile)
	require.NoError(t, err)
	require.Equal(t, emptyRoutesYAML, string(contents), "source config must remain a valid empty route map")
	require.Equal(t, 0, p.preparation.DesiredRoutes)
	require.Empty(t, p.preparation.FailedRoutes, "an intentional empty route map must be a clean provider load")
	require.Zero(t, p.NumRoutes())
	assertConfiguredRoutesVisible(t, root, p, ep, []string{}, "after clean empty route file removes all routes")
}
