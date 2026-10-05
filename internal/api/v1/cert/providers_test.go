package certapi

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/autocert"
)

func TestProvidersReturnsSortedRegistryAndBuiltins(t *testing.T) {
	previous := autocert.Providers
	autocert.Providers = map[string]autocert.Generator{
		"spaceship":             nil,
		"rfc2136":               nil,
		"dnsupdate":             nil,
		autocert.ProviderLocal:  nil,
		autocert.ProviderPseudo: nil,
	}
	t.Cleanup(func() { autocert.Providers = previous })

	providers := callProviders(t)
	require.Equal(t, []string{"custom", "dnsupdate", "local", "rfc2136", "spaceship"}, providers)
}

func TestProvidersDoesNotDuplicateRegisteredCustom(t *testing.T) {
	previous := autocert.Providers
	autocert.Providers = map[string]autocert.Generator{
		"spaceship":             nil,
		autocert.ProviderCustom: nil,
	}
	t.Cleanup(func() { autocert.Providers = previous })

	providers := callProviders(t)
	require.Equal(t, []string{"custom", "spaceship"}, providers)
}

func callProviders(t *testing.T) []string {
	t.Helper()

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest("GET", "/api/v1/cert/providers", nil)
	Providers(context)

	require.Equal(t, 200, recorder.Code)
	var providers []string
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &providers))
	return providers
}
