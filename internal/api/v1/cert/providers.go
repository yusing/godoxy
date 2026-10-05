package certapi

import (
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/autocert"
	_ "github.com/yusing/goutils/apitypes"
)

// @x-id       "providers"
// @BasePath   /api/v1
// @Summary    List certificate providers
// @Description List available DNS certificate provider IDs
// @Tags       cert
// @Produce    json
// @Success    200 {array} string
// @Failure    403 {object} apitypes.ErrorResponse "Unauthorized"
// @Router     /cert/providers [get]
func Providers(c *gin.Context) {
	providers := make([]string, 0, len(autocert.Providers)+1)
	for provider := range autocert.Providers {
		if provider != autocert.ProviderPseudo {
			providers = append(providers, provider)
		}
	}
	if _, exists := autocert.Providers[autocert.ProviderCustom]; !exists {
		providers = append(providers, autocert.ProviderCustom)
	}
	sort.Strings(providers)

	c.JSON(http.StatusOK, providers)
}
