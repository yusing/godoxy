package agentapi

import (
	"net"
	"net/http"
	"strconv"

	_ "embed"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/agent/pkg/agent"
	apitypes "github.com/yusing/goutils/apitypes"
)

type NewAgentRequest struct {
	Name             string                 `json:"name" binding:"required"`
	Host             string                 `json:"host" binding:"required"`
	Port             int                    `json:"port" binding:"required,min=1,max=65535"`
	Type             string                 `json:"type" binding:"required,oneof=docker system"`
	Nightly          bool                   `json:"nightly" binding:"omitempty"`
	ContainerRuntime agent.ContainerRuntime `json:"container_runtime" binding:"omitempty,oneof=docker podman" default:"docker"`
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
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}

	hostport := net.JoinHostPort(request.Host, strconv.Itoa(request.Port))
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
		Name:             request.Name,
		Port:             request.Port,
		CACert:           ca.String(),
		SSLCert:          srv.String(),
		ContainerRuntime: request.ContainerRuntime,
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
