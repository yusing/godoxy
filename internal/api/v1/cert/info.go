package certapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/autocert"
	apitypes "github.com/yusing/goutils/apitypes"
)

// @x-id				"info"
// @BasePath		/api/v1
// @Summary		Get cert info
// @Description	Get cert info
// @Tags			cert
// @Produce		json
// @Success		200	{array}	  autocert.CertInfo
// @Failure		403	{object}	apitypes.ErrorResponse "Unauthorized"
// @Failure		404	{object}	apitypes.ErrorResponse "No certificates found or autocert is not enabled"
// @Failure		500	{object}	apitypes.ErrorResponse "Internal server error"
// @Router		/cert/info [get]
func Info(c *gin.Context) {
	provider := autocert.ActiveProvider.Load()
	if provider == nil {
		c.JSON(http.StatusNotFound, apitypes.Error("autocert is not enabled"))
		return
	}

	certInfos, err := provider.GetCertInfos()
	if err != nil {
		if errors.Is(err, autocert.ErrNoCertificates) {
			c.JSON(http.StatusNotFound, apitypes.Error("no certificate found"))
			return
		}
		c.Error(apitypes.InternalServerError(err, "failed to get cert info"))
		return
	}

	c.JSON(http.StatusOK, certInfos)
}
