package certapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	apitypes "github.com/yusing/godoxy/internal/api/types"
	"github.com/yusing/godoxy/internal/autocert"
)

type CertInfo struct {
	Subject        string   `json:"subject"`
	Issuer         string   `json:"issuer"`
	NotBefore      int64    `json:"not_before"`
	NotAfter       int64    `json:"not_after"`
	DNSNames       []string `json:"dns_names"`
	EmailAddresses []string `json:"email_addresses"`
} // @name CertInfo

// @x-id				"info"
// @BasePath		/api/v1
// @Summary		Get cert info
// @Description	Get cert info
// @Tags			cert
// @Produce		json
// @Success		200	{object}	CertInfo
// @Failure		403	{object}	apitypes.ErrorResponse
// @Failure		404	{object}	apitypes.ErrorResponse
// @Failure		500	{object}	apitypes.ErrorResponse
// @Router			/cert/info [get]
func Info(c *gin.Context) {
	autocert := autocert.ActiveProvider.Load()
	if autocert == nil {
		c.JSON(http.StatusNotFound, apitypes.Error("autocert is not enabled"))
		return
	}

	cert, err := autocert.GetCert(nil)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to get cert info"))
		return
	}

	certInfo := CertInfo{
		Subject:        cert.Leaf.Subject.CommonName,
		Issuer:         cert.Leaf.Issuer.CommonName,
		NotBefore:      cert.Leaf.NotBefore.Unix(),
		NotAfter:       cert.Leaf.NotAfter.Unix(),
		DNSNames:       cert.Leaf.DNSNames,
		EmailAddresses: cert.Leaf.EmailAddresses,
	}
	c.JSON(http.StatusOK, certInfo)
}
