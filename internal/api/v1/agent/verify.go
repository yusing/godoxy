package agentapi

import (
	"fmt"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/agent/pkg/certs"
	config "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/route/provider"
	apitypes "github.com/yusing/goutils/apitypes"
	gperr "github.com/yusing/goutils/errs"
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
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}

	filename, ok := certs.AgentCertsFilepath(request.Host)
	if !ok {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid host", nil))
		return
	}

	ca, err := fromEncryptedPEMPairResponse(request.CA)
	if err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid CA", err))
		return
	}

	client, err := fromEncryptedPEMPairResponse(request.Client)
	if err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid client", err))
		return
	}

	nRoutesAdded, err := verifyNewAgent(request.Host, ca, client, request.ContainerRuntime)
	if err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}

	zip, err := certs.ZipCert(ca.Cert, client.Cert, client.Key)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to zip certs"))
		return
	}

	if err := os.WriteFile(filename, zip, 0o600); err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to write certs"))
		return
	}

	c.JSON(http.StatusOK, apitypes.Success(fmt.Sprintf("Added %d routes", nRoutesAdded)))
}

func verifyNewAgent(host string, ca agent.PEMPair, client agent.PEMPair, containerRuntime agent.ContainerRuntime) (int, gperr.Error) {
	cfgState := config.ActiveState.Load()
	for _, a := range cfgState.Value().Providers.Agents {
		if a.Addr == host {
			return 0, gperr.New("agent already exists")
		}
	}

	var agentCfg agent.AgentConfig
	agentCfg.Addr = host
	agentCfg.Runtime = containerRuntime

	err := agentCfg.StartWithCerts(cfgState.Context(), ca.Cert, client.Cert, client.Key)
	if err != nil {
		return 0, gperr.Wrap(err, "failed to start agent")
	}

	provider := provider.NewAgentProvider(&agentCfg)
	if _, loaded := cfgState.LoadOrStoreProvider(provider.String(), provider); loaded {
		return 0, gperr.Errorf("provider %s already exists", provider.String())
	}

	// agent must be added before loading routes
	agent.AddAgent(&agentCfg)
	err = provider.LoadRoutes()
	if err != nil {
		cfgState.DeleteProvider(provider.String())
		agent.RemoveAgent(&agentCfg)
		return 0, gperr.Wrap(err, "failed to load routes")
	}

	return provider.NumRoutes(), nil
}
