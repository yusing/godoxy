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
	"github.com/yusing/godoxy/internal/routing"
	apitypes "github.com/yusing/goutils/apitypes"
	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/task"
)

type VerifyNewAgentRequest struct {
	Host             string                 `json:"host"`
	CA               PEMPairResponse        `json:"ca"`
	Client           PEMPairResponse        `json:"client"`
	ContainerRuntime agent.ContainerRuntime `json:"container_runtime"`
	AddToConfig      bool                   `json:"add_to_config,omitempty" extensions:"x-omitempty"`
} // @name VerifyNewAgentRequest

type VerifyNewAgentResponse struct {
	Message    string                     `json:"message"`
	Agents     []*agent.AgentConfig       `json:"agents"`
	Activation routing.ProviderActivation `json:"activation"`
} // @name VerifyNewAgentResponse

var (
	verifyStartNewAgentFunc          = verifyStartNewAgent
	persistAgentHostToConfigFunc     = persistAgentHostToConfig
	suppressNextConfigReloadFunc     = configtypes.SuppressNextConfigReload
	clearConfigReloadSuppressionFunc = configtypes.ClearConfigReloadSuppression
	zipCertFunc                      = certs.ZipCert
	writeAgentCertZipFunc            = func(filename string, zip []byte) error { return os.WriteFile(filename, zip, 0o600) }
	listAgentsFunc                   = listAgentConfigs
	initAgentConfigWithCertsFunc     = (*agent.AgentConfig).InitWithCerts
	newAgentProviderFunc             = func(cfg *agent.AgentConfig) routing.Provider {
		return provider.NewAgentProvider(cfg)
	}
)

func listAgentConfigs(ctx context.Context) []*agent.AgentConfig {
	agents := agentpool.FromCtx(ctx)
	if agents == nil {
		return nil
	}
	entries := agents.List()
	configs := make([]*agent.AgentConfig, len(entries))
	for i, agentInfo := range entries {
		configs[i] = agentInfo.AgentConfig
	}
	return configs
}

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
// @Failure		409		{object}	ErrorResponse
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

	ctx := c.Request.Context()
	runtimeState := configtypes.FromCtx(ctx)
	coordinator := configtypes.RuntimeMutationCoordinatorFromCtx(ctx)
	if runtimeState == nil || coordinator == nil {
		c.JSON(http.StatusConflict, apitypes.Error("runtime unavailable", nil))
		return
	}
	release, err := coordinator.BeginRuntimeMutation(runtimeState)
	if err != nil {
		c.JSON(http.StatusConflict, apitypes.Error("runtime changed", err))
		return
	}
	defer release()

	activation, cleanupStartedAgent, err := verifyStartNewAgentFunc(ctx, runtimeState, request.Host, ca, client, request.ContainerRuntime)
	if err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}

	zip, err := zipCertFunc(ca.Cert, client.Cert, client.Key)
	if err != nil {
		cleanupStartedAgent(err)
		c.Error(apitypes.InternalServerError(err, "failed to zip certs"))
		return
	}

	if err := writeAgentCertZipFunc(filename, zip); err != nil {
		cleanupStartedAgent(err)
		c.Error(apitypes.InternalServerError(err, "failed to write certs"))
		return
	}

	if request.AddToConfig {
		suppressNextConfigReloadFunc()
		if err := persistAgentHostToConfigFunc(request.Host); err != nil {
			clearConfigReloadSuppressionFunc()
			cleanupStartedAgent(err)
			c.Error(apitypes.InternalServerError(err, "failed to update config"))
			return
		}
	}

	message := fmt.Sprintf("Added %d routes", activation.ActiveRoutes)
	if len(activation.FailedRoutes) > 0 {
		message = fmt.Sprintf("Added %d routes, %d failed", activation.ActiveRoutes, len(activation.FailedRoutes))
	}
	c.JSON(http.StatusOK, VerifyNewAgentResponse{
		Message:    message,
		Agents:     listAgentsFunc(ctx),
		Activation: activation,
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

func verifyStartNewAgent(ctx context.Context, cfgState configtypes.State, host string, ca agent.PEMPair, client agent.PEMPair, containerRuntime agent.ContainerRuntime) (routing.ProviderActivation, func(reason any), error) {
	if cfgState == nil {
		return routing.ProviderActivation{}, nil, errors.New("runtime state not found in request context")
	}
	if cause := context.Cause(ctx); cause != nil {
		return routing.ProviderActivation{}, nil, cause
	}
	var agentCfg agent.AgentConfig
	agentCfg.Addr = host
	agentCfg.Runtime = containerRuntime

	// check if agent host exists in the config
	agents := agentpool.FromCtx(cfgState.Context())
	if agents == nil {
		return routing.ProviderActivation{}, nil, errors.New("agent pool not initialized")
	}
	for _, a := range cfgState.Value().Providers.Agents {
		if a.Addr == host {
			return routing.ProviderActivation{}, nil, errAgentAlreadyExists
		}
	}
	// check if agent host exists in the agent pool
	if agents.Has(&agentCfg) {
		return routing.ProviderActivation{}, nil, errAgentAlreadyExists
	}

	err := initAgentConfigWithCertsFunc(&agentCfg, ctx, ca.Cert, client.Cert, client.Key)
	if err != nil {
		return routing.ProviderActivation{}, nil, fmt.Errorf("failed to initialize agent config: %w", err)
	}

	provider := newAgentProviderFunc(&agentCfg)
	if _, loaded := cfgState.LoadOrStoreProvider(provider.String(), provider); loaded {
		return routing.ProviderActivation{}, nil, fmt.Errorf("provider %s already exists", provider.String())
	}

	agentAdded := false
	var providerTaskParent *task.Task
	cleanupProvider := func(reason any) {
		if providerTaskParent != nil {
			providerTaskParent.FinishAndWait(reason)
		}
		cfgState.DeleteProvider(provider.String())
		if agentAdded {
			agents.Remove(&agentCfg)
		}
	}

	// agent must be added before loading routes
	added := agents.Add(&agentCfg)
	if !added {
		cleanupProvider(errAgentAlreadyExists)
		return routing.ProviderActivation{}, nil, errAgentAlreadyExists
	}
	agentAdded = true
	loadErr := provider.LoadRoutes(ctx)

	providerTaskParent = cfgState.Task().Subtask("verify_agent."+provider.String(), false)
	activation := provider.Activate(providerTaskParent)
	// Provider.LoadRoutes can retain valid routes while returning validation
	// errors for invalid siblings. Provider implementations normally carry
	// those details into activation; preserve an otherwise unrepresented load
	// failure so a zero-active result still fails closed.
	if loadErr != nil && activation.InfrastructureError == nil && len(activation.FailedRoutes) == 0 {
		activation.InfrastructureError = gperr.Wrap(loadErr)
	}
	var activationErrs []error
	if activation.InfrastructureError != nil {
		activationErrs = append(activationErrs, activation.InfrastructureError)
	}
	for _, route := range activation.FailedRoutes {
		activationErrs = append(activationErrs, route.Err)
	}
	err = errors.Join(activationErrs...)
	if err != nil && activation.ActiveRoutes == 0 {
		cleanupProvider(err)
		return activation, nil, fmt.Errorf("failed to start routes: %w", err)
	}

	return activation, cleanupProvider, nil
}
