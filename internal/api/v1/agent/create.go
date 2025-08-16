package agentapi

import (
	"fmt"
	"net/http"

	_ "embed"

	"github.com/gin-gonic/gin"
	"github.com/yusing/go-proxy/agent/pkg/agent"
	apitypes "github.com/yusing/go-proxy/internal/api/types"
)

type NewAgentRequest struct {
	Name    string `form:"name" validate:"required"`
	Host    string `form:"host" validate:"required"`
	Port    int    `form:"port" validate:"required,min=1,max=65535"`
	Type    string `form:"type" validate:"required,oneof=docker system"`
	Nightly bool   `form:"nightly" validate:"omitempty"`
} // @name NewAgentRequest

type NewAgentResponse struct {
	Compose string          `json:"compose"`
	CA      PEMPairResponse `json:"ca"`
	Client  PEMPairResponse `json:"client"`
} // @name NewAgentResponse

// @x-id				"create"
// @BasePath		/api/v1
// @Summary		Create a new agent
// @Description	Create a new agent and return the docker compose file, encrypted CA and client PEMs
// @Description	The returned PEMs are encrypted with a random key and will be used for verification when adding a new agent
// @Tags			agent
// @Accept			json
// @Produce		json
// @Param			request	body		NewAgentRequest	true	"Request"
// @Success		200		{object}	NewAgentResponse
// @Failure		400		{object}	apitypes.ErrorResponse
// @Failure		403		{object}	apitypes.ErrorResponse
// @Failure		409		{object}	apitypes.ErrorResponse
// @Failure		500		{object}	apitypes.ErrorResponse
// @Router			/agent/create [post]
func Create(c *gin.Context) {
	var request NewAgentRequest
	if err := c.ShouldBindQuery(&request); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}
	hostport := fmt.Sprintf("%s:%d", request.Host, request.Port)
	if _, ok := agent.GetAgent(hostport); ok {
		c.JSON(http.StatusConflict, apitypes.Error("agent already exists"))
		return
	}

	var image string
	if request.Nightly {
		image = agent.DockerImageNightly
	} else {
		image = agent.DockerImageProduction
	}

	ca, srv, client, err := agent.NewAgent()
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to create agent"))
		return
	}

	var cfg agent.Generator = &agent.AgentEnvConfig{
		Name:    request.Name,
		Port:    request.Port,
		CACert:  ca.String(),
		SSLCert: srv.String(),
	}
	if request.Type == "docker" {
		cfg = &agent.AgentComposeConfig{
			Image:          image,
			AgentEnvConfig: cfg.(*agent.AgentEnvConfig),
		}
	}
	template, err := cfg.Generate()
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to generate agent config"))
		return
	}

	key := getEncryptionKey()
	encCA, err := ca.Encrypt(key)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to encrypt CA PEMs"))
		return
	}
	encClient, err := client.Encrypt(key)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to encrypt client PEMs"))
		return
	}

	c.JSON(http.StatusOK, NewAgentResponse{
		Compose: template,
		CA:      toPEMPairResponse(encCA),
		Client:  toPEMPairResponse(encClient),
	})
}
