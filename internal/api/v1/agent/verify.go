package agentapi

import (
	"fmt"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/agent/pkg/certs"
	. "github.com/yusing/go-proxy/internal/api/types"
	config "github.com/yusing/go-proxy/internal/config/types"
)

type VerifyNewAgentRequest struct {
	Host             string                 `json:"host"`
	CA               PEMPairResponse        `json:"ca"`
	Client           PEMPairResponse        `json:"client"`
	ContainerRuntime agent.ContainerRuntime `json:"container_runtime"`
} // @name VerifyNewAgentRequest

// @x-id          "verify"
// @BasePath		/api/v1
// @Summary		Verify a new agent
// @Description	Verify a new agent and return the number of routes added
// @Tags			agent
// @Accept			json
// @Produce		json
// @Param			request	body		VerifyNewAgentRequest	true	"Request"
// @Success		200		{object}	SuccessResponse
// @Failure		400		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Router			/agent/verify [post]
func Verify(c *gin.Context) {
	var request VerifyNewAgentRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, Error("invalid request", err))
		return
	}

	filename, ok := certs.AgentCertsFilepath(request.Host)
	if !ok {
		c.JSON(http.StatusBadRequest, Error("invalid host", nil))
		return
	}

	ca, err := fromEncryptedPEMPairResponse(request.CA)
	if err != nil {
		c.JSON(http.StatusBadRequest, Error("invalid CA", err))
		return
	}

	client, err := fromEncryptedPEMPairResponse(request.Client)
	if err != nil {
		c.JSON(http.StatusBadRequest, Error("invalid client", err))
		return
	}

	nRoutesAdded, err := config.GetInstance().VerifyNewAgent(request.Host, ca, client, request.ContainerRuntime)
	if err != nil {
		c.JSON(http.StatusBadRequest, Error("invalid request", err))
		return
	}

	zip, err := certs.ZipCert(ca.Cert, client.Cert, client.Key)
	if err != nil {
		c.Error(InternalServerError(err, "failed to zip certs"))
		return
	}

	if err := os.WriteFile(filename, zip, 0o600); err != nil {
		c.Error(InternalServerError(err, "failed to write certs"))
		return
	}

	c.JSON(http.StatusOK, Success(fmt.Sprintf("Added %d routes", nRoutesAdded)))
}
