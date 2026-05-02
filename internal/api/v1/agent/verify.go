package agentapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"

	"github.com/gin-gonic/gin"
	"github.com/goccy/go-yaml"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/agent/pkg/certs"
	"github.com/yusing/godoxy/internal/agentpool"
	"github.com/yusing/godoxy/internal/common"
	configtypes "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/route/provider"
	apitypes "github.com/yusing/goutils/apitypes"
)

type VerifyNewAgentRequest struct {
	Host             string                 `json:"host"`
	CA               PEMPairResponse        `json:"ca"`
	Client           PEMPairResponse        `json:"client"`
	ContainerRuntime agent.ContainerRuntime `json:"container_runtime"`
	AddToConfig      bool                   `json:"add_to_config,omitempty"`
} // @name VerifyNewAgentRequest

type VerifyNewAgentResponse struct {
	Message string               `json:"message"`
	Agents  []*agent.AgentConfig `json:"agents"`
} // @name VerifyNewAgentResponse

var (
	verifyNewAgentFunc               = verifyNewAgent
	persistAgentHostToConfigFunc     = persistAgentHostToConfig
	suppressNextConfigReloadFunc     = configtypes.SuppressNextConfigReload
	clearConfigReloadSuppressionFunc = configtypes.ClearConfigReloadSuppression
	zipCertFunc                      = certs.ZipCert
	writeAgentCertZipFunc            = func(filename string, zip []byte) error { return os.WriteFile(filename, zip, 0o600) }
	listAgentsFunc                   = listAgentConfigs
)

// @x-id          "verify"
// @BasePath		/api/v1
// @Summary		Verify a new agent
// @Description	Verify a new agent and return the number of routes added
// @Tags			agent
// @Accept			json
// @Produce		json
// @Param			request	body		VerifyNewAgentRequest	true	"Request"
// @Success		200		{object}	VerifyNewAgentResponse
// @Failure		400		{object}	ErrorResponse
// @Failure		403		{object}	ErrorResponse
// @Failure		500		{object}	ErrorResponse
// @Router			/agent/verify [post]
func Verify(c *gin.Context) {
	// avoid timeout waiting for response headers
	c.Status(http.StatusContinue)

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

	nRoutesAdded, err := verifyNewAgentFunc(c.Request.Context(), request.Host, ca, client, request.ContainerRuntime)
	if err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}

	zip, err := zipCertFunc(ca.Cert, client.Cert, client.Key)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to zip certs"))
		return
	}

	if err := writeAgentCertZipFunc(filename, zip); err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to write certs"))
		return
	}

	if request.AddToConfig {
		suppressNextConfigReloadFunc()
		if err := persistAgentHostToConfigFunc(request.Host); err != nil {
			clearConfigReloadSuppressionFunc()
			c.Error(apitypes.InternalServerError(err, "failed to update config"))
			return
		}
	}

	c.JSON(http.StatusOK, VerifyNewAgentResponse{
		Message: fmt.Sprintf("Added %d routes", nRoutesAdded),
		Agents:  listAgentsFunc(),
	})
}

var errAgentAlreadyExists = errors.New("agent already exists")

func persistAgentHostToConfig(host string) error {
	configDoc, err := loadConfigDocument(common.ConfigPath)
	if err != nil {
		return err
	}
	changed, err := appendAgentHostToConfigDocument(configDoc, host)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}

	data, err := yaml.Marshal(configDoc)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := os.WriteFile(common.ConfigPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	return nil
}

func loadConfigDocument(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}

	var configDoc map[string]any
	if err := yaml.Unmarshal(data, &configDoc); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	if configDoc == nil {
		return map[string]any{}, nil
	}
	return configDoc, nil
}

func appendAgentHostToConfigDocument(configDoc map[string]any, host string) (bool, error) {
	providersValue, ok := configDoc["providers"]
	if !ok || providersValue == nil {
		configDoc["providers"] = map[string]any{
			"agents": []string{host},
		}
		return true, nil
	}

	providers, ok := providersValue.(map[string]any)
	if !ok {
		return false, fmt.Errorf("providers must be a mapping, got %T", providersValue)
	}

	agentsValue, ok := providers["agents"]
	if !ok || agentsValue == nil {
		providers["agents"] = []string{host}
		return true, nil
	}

	switch agents := agentsValue.(type) {
	case []any:
		for _, entry := range agents {
			if configAgentEntryMatchesHost(entry, host) {
				return false, nil
			}
		}
		providers["agents"] = append(agents, host)
		return true, nil
	case []string:
		if slices.Contains(agents, host) {
			return false, nil
		}
		providers["agents"] = append(agents, host)
		return true, nil
	default:
		return false, fmt.Errorf("providers.agents must be a list, got %T", agentsValue)
	}
}

func configAgentEntryMatchesHost(entry any, host string) bool {
	switch value := entry.(type) {
	case string:
		return value == host
	case map[string]any:
		addr, _ := value["addr"].(string)
		return addr == host
	default:
		return false
	}
}

func listAgentConfigs() []*agent.AgentConfig {
	agents := agentpool.List()
	configs := make([]*agent.AgentConfig, 0, len(agents))
	for _, agentInfo := range agents {
		configs = append(configs, agentInfo.AgentConfig)
	}
	return configs
}

func verifyNewAgent(ctx context.Context, host string, ca agent.PEMPair, client agent.PEMPair, containerRuntime agent.ContainerRuntime) (int, error) {
	var agentCfg agent.AgentConfig
	agentCfg.Addr = host
	agentCfg.Runtime = containerRuntime

	// check if agent host exists in the config
	cfgState := configtypes.ActiveState.Load()
	for _, a := range cfgState.Value().Providers.Agents {
		if a.Addr == host {
			return 0, errAgentAlreadyExists
		}
	}
	// check if agent host exists in the agent pool
	if agentpool.Has(&agentCfg) {
		return 0, errAgentAlreadyExists
	}

	err := agentCfg.InitWithCerts(ctx, ca.Cert, client.Cert, client.Key)
	if err != nil {
		return 0, fmt.Errorf("failed to initialize agent config: %w", err)
	}

	provider := provider.NewAgentProvider(&agentCfg)
	if _, loaded := cfgState.LoadOrStoreProvider(provider.String(), provider); loaded {
		return 0, fmt.Errorf("provider %s already exists", provider.String())
	}

	// agent must be added before loading routes
	added := agentpool.Add(&agentCfg)
	if !added {
		return 0, errAgentAlreadyExists
	}
	err = provider.LoadRoutes()
	if err != nil {
		cfgState.DeleteProvider(provider.String())
		agentpool.Remove(&agentCfg)
		return 0, fmt.Errorf("failed to load routes: %w", err)
	}

	return provider.NumRoutes(), nil
}
