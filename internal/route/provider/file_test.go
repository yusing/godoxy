package provider

import (
	"net/http"
	"net/http/httptest"
	"testing"

	_ "embed"

	"github.com/stretchr/testify/require"
	config "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/health"
	"github.com/yusing/goutils/task"
)

//go:embed fixtures/all_fields.yaml
var testAllFieldsYAML []byte

func TestFile(t *testing.T) {
	_, err := validate(testAllFieldsYAML)
	require.NoError(t, err)
}

func TestFileSilenceDefaults(t *testing.T) {
	parent := task.GetTestTask(t)
	state := dockerDefaultsState{}
	var window health.SilenceWindow
	require.NoError(t, window.Parse("02:00-04:00"))
	state.cfg.Defaults.HealthCheck.Silence = []health.SilenceWindow{window}
	config.SetCtx(parent, state)
	routes, err := validate([]byte(`inherit:
  host: example.com
optout:
  host: example.com
  healthcheck:
    silence: []
override:
  host: example.com
  healthcheck:
    silence: [23:00-01:00]
`))
	require.NoError(t, err)
	for alias, r := range routes {
		r.Alias = alias
		require.NoError(t, r.ValidateContext(parent.Context()))
	}
	require.Equal(t, state.cfg.Defaults.HealthCheck.Silence, routes["inherit"].HealthCheck.Silence)
	require.NotNil(t, routes["optout"].HealthCheck.Silence)
	require.Empty(t, routes["optout"].HealthCheck.Silence)
	require.Len(t, routes["override"].HealthCheck.Silence, 1)
	require.Equal(t, "23:00-01:00", routes["override"].HealthCheck.Silence[0].String())
}

func TestFileRuleEnvironment(t *testing.T) {
	t.Setenv("GODOXY_RULE_ALIAS", "app")
	t.Setenv("GODOXY_RULE_HOST", "localhost")
	t.Setenv("GODOXY_RULE_MESSAGE", "say \"hello\" {text}\nnext line")
	routes, err := validate([]byte(`${RULE_ALIAS}:
  host: ${RULE_HOST}
  port: 8080
  rules: |
    default { error 403 "${RULE_MESSAGE}" }
`))
	require.NoError(t, err)
	require.Contains(t, routes, "app")
	require.Equal(t, "localhost", routes["app"].Host)
	w := httptest.NewRecorder()
	routes["app"].Rules.BuildHandler(func(http.ResponseWriter, *http.Request) {
		t.Fatal("upstream called for a terminating rule")
	})(w, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusForbidden, w.Code)
	require.Equal(t, "say \"hello\" {text}\nnext line\n", w.Body.String())
}

func TestValidateIdlewatcherUsesRouteProxmoxBinding(t *testing.T) {
	data := []byte(`app:
  host: example.com
  proxmox:
    node: pve
    vmid: 119
  idlewatcher:
    idle_timeout: 30m
`)

	require.NoError(t, Validate(t.Context(), data))
}

func TestValidateIdlewatcherRequiresBoundProvider(t *testing.T) {
	data := []byte(`app:
  host: example.com
  idlewatcher:
    idle_timeout: 30m
`)

	err := Validate(t.Context(), data)
	require.ErrorContains(t, err, "missing idlewatcher provider config")
}
