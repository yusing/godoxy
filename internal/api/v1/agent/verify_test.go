package agentapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"iter"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/internal/agentpool"
	configtypes "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/routing"
	"github.com/yusing/goutils/server"
	"github.com/yusing/goutils/synk"
	"github.com/yusing/goutils/task"
)

func TestAppendAgentHostToConfigDocument(t *testing.T) {
	t.Run("creates providers agents when absent", func(t *testing.T) {
		configDoc := map[string]any{
			"entrypoint": map[string]any{
				"websecure": true,
			},
		}

		changed, err := appendAgentHostToConfigDocument(configDoc, "10.0.0.1:8890")
		require.NoError(t, err)
		require.True(t, changed)
		require.Equal(t, map[string]any{
			"entrypoint": map[string]any{
				"websecure": true,
			},
			"providers": map[string]any{
				"agents": []string{"10.0.0.1:8890"},
			},
		}, configDoc)
	})

	t.Run("appends host once", func(t *testing.T) {
		configDoc := map[string]any{
			"providers": map[string]any{
				"agents": []any{"10.0.0.1:8890"},
			},
		}

		changed, err := appendAgentHostToConfigDocument(configDoc, "10.0.0.2:8890")
		require.NoError(t, err)
		require.True(t, changed)
		require.Equal(t, []any{"10.0.0.1:8890", "10.0.0.2:8890"}, configDoc["providers"].(map[string]any)["agents"])
	})

	t.Run("does not duplicate existing host", func(t *testing.T) {
		configDoc := map[string]any{
			"providers": map[string]any{
				"agents": []any{
					"10.0.0.1:8890",
					map[string]any{"addr": "10.0.0.2:8890"},
				},
			},
		}

		changed, err := appendAgentHostToConfigDocument(configDoc, "10.0.0.2:8890")
		require.NoError(t, err)
		require.False(t, changed)
		require.Len(t, configDoc["providers"].(map[string]any)["agents"], 2)
	})
}

func TestVerifyReturnsManagedResponseAndSkipsConfigPersistence(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previousVerifyNewAgentFunc := verifyStartNewAgentFunc
	previousPersistAgentHostToConfigFunc := persistAgentHostToConfigFunc
	previousSuppressNextConfigReloadFunc := suppressNextConfigReloadFunc
	previousClearConfigReloadSuppressionFunc := clearConfigReloadSuppressionFunc
	previousWriteAgentCertZipFunc := writeAgentCertZipFunc
	previousListAgentsFunc := listAgentsFunc
	t.Cleanup(func() {
		verifyStartNewAgentFunc = previousVerifyNewAgentFunc
		persistAgentHostToConfigFunc = previousPersistAgentHostToConfigFunc
		suppressNextConfigReloadFunc = previousSuppressNextConfigReloadFunc
		clearConfigReloadSuppressionFunc = previousClearConfigReloadSuppressionFunc
		writeAgentCertZipFunc = previousWriteAgentCertZipFunc
		listAgentsFunc = previousListAgentsFunc
	})

	verifyNewAgentCalls := 0
	verifyStartNewAgentFunc = func(ctx context.Context, host string, ca agent.PEMPair, client agent.PEMPair, containerRuntime agent.ContainerRuntime) (int, func(any), error) {
		verifyNewAgentCalls++
		require.Equal(t, "10.0.0.1:8890", host)
		require.Equal(t, []byte("ca-cert"), ca.Cert)
		require.Equal(t, []byte("client-cert"), client.Cert)
		require.Equal(t, []byte("client-key"), client.Key)
		require.Equal(t, agent.ContainerRuntimeDocker, containerRuntime)
		return 2, func(any) {}, nil
	}

	persistCalls := 0
	persistAgentHostToConfigFunc = func(string) error {
		persistCalls++
		return nil
	}

	suppressCalls := 0
	suppressNextConfigReloadFunc = func() {
		suppressCalls++
	}

	clearCalls := 0
	clearConfigReloadSuppressionFunc = func() {
		clearCalls++
	}

	writeAgentCertZipCalls := 0
	writeAgentCertZipFunc = func(filename string, zip []byte) error {
		writeAgentCertZipCalls++
		require.Equal(t, "certs/10.0.0.1:8890.zip", filename)
		require.NotEmpty(t, zip)
		return nil
	}

	listAgentsFunc = func() []*agent.AgentConfig {
		return []*agent.AgentConfig{
			{
				AgentInfo: agent.AgentInfo{
					Name:    "agent-1",
					Runtime: agent.ContainerRuntimeDocker,
				},
				Addr: "10.0.0.1:8890",
			},
		}
	}

	caPair := agent.PEMPair{Cert: []byte("ca-cert"), Key: []byte("ca-key")}
	encCA, err := caPair.Encrypt(getEncryptionKey())
	require.NoError(t, err)
	clientPair := agent.PEMPair{Cert: []byte("client-cert"), Key: []byte("client-key")}
	encClient, err := clientPair.Encrypt(getEncryptionKey())
	require.NoError(t, err)

	body, err := json.Marshal(VerifyNewAgentRequest{
		Host:             "10.0.0.1:8890",
		CA:               toPEMPairResponse(encCA),
		Client:           toPEMPairResponse(encClient),
		ContainerRuntime: agent.ContainerRuntimeDocker,
		AddToConfig:      false,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/verify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = req

	Verify(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 1, verifyNewAgentCalls)
	require.Equal(t, 0, persistCalls)
	require.Equal(t, 0, suppressCalls)
	require.Equal(t, 0, clearCalls)
	require.Equal(t, 1, writeAgentCertZipCalls)

	var response struct {
		Message string `json:"message"`
		Agents  []struct {
			Name string `json:"name"`
			Addr string `json:"addr"`
		} `json:"agents"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "Added 2 routes", response.Message)
	require.Len(t, response.Agents, 1)
	require.Equal(t, "agent-1", response.Agents[0].Name)
	require.Equal(t, "10.0.0.1:8890", response.Agents[0].Addr)
}

func TestVerifyClearsConfigReloadSuppressionWhenConfigPersistenceFails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previousVerifyNewAgentFunc := verifyStartNewAgentFunc
	previousPersistAgentHostToConfigFunc := persistAgentHostToConfigFunc
	previousSuppressNextConfigReloadFunc := suppressNextConfigReloadFunc
	previousClearConfigReloadSuppressionFunc := clearConfigReloadSuppressionFunc
	previousWriteAgentCertZipFunc := writeAgentCertZipFunc
	previousListAgentsFunc := listAgentsFunc
	t.Cleanup(func() {
		verifyStartNewAgentFunc = previousVerifyNewAgentFunc
		persistAgentHostToConfigFunc = previousPersistAgentHostToConfigFunc
		suppressNextConfigReloadFunc = previousSuppressNextConfigReloadFunc
		clearConfigReloadSuppressionFunc = previousClearConfigReloadSuppressionFunc
		writeAgentCertZipFunc = previousWriteAgentCertZipFunc
		listAgentsFunc = previousListAgentsFunc
	})

	verifyNewAgentCalls := 0
	cleanupCalls := 0
	verifyStartNewAgentFunc = func(ctx context.Context, host string, ca agent.PEMPair, client agent.PEMPair, containerRuntime agent.ContainerRuntime) (int, func(any), error) {
		verifyNewAgentCalls++
		require.Equal(t, "10.0.0.1:8890", host)
		return 2, func(reason any) {
			cleanupCalls++
			require.EqualError(t, reason.(error), "persist failed")
		}, nil
	}

	writeAgentCertZipCalls := 0
	writeAgentCertZipFunc = func(filename string, zip []byte) error {
		writeAgentCertZipCalls++
		require.Equal(t, "certs/10.0.0.1:8890.zip", filename)
		require.NotEmpty(t, zip)
		return nil
	}

	persistCalls := 0
	persistAgentHostToConfigFunc = func(host string) error {
		persistCalls++
		require.Equal(t, "10.0.0.1:8890", host)
		return errors.New("persist failed")
	}

	suppressCalls := 0
	suppressNextConfigReloadFunc = func() {
		suppressCalls++
	}

	clearCalls := 0
	clearConfigReloadSuppressionFunc = func() {
		clearCalls++
	}

	listAgentsFunc = func() []*agent.AgentConfig {
		require.Fail(t, "listAgentsFunc should not be called after config persistence failure")
		return nil
	}

	caPair := agent.PEMPair{Cert: []byte("ca-cert"), Key: []byte("ca-key")}
	encCA, err := caPair.Encrypt(getEncryptionKey())
	require.NoError(t, err)
	clientPair := agent.PEMPair{Cert: []byte("client-cert"), Key: []byte("client-key")}
	encClient, err := clientPair.Encrypt(getEncryptionKey())
	require.NoError(t, err)

	body, err := json.Marshal(VerifyNewAgentRequest{
		Host:             "10.0.0.1:8890",
		CA:               toPEMPairResponse(encCA),
		Client:           toPEMPairResponse(encClient),
		ContainerRuntime: agent.ContainerRuntimeDocker,
		AddToConfig:      true,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/verify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = req

	Verify(c)

	require.Equal(t, 1, verifyNewAgentCalls)
	require.Equal(t, 1, writeAgentCertZipCalls)
	require.Equal(t, 1, persistCalls)
	require.Equal(t, 1, suppressCalls)
	require.Equal(t, 1, clearCalls)
	require.Equal(t, 1, cleanupCalls)
	require.Len(t, c.Errors, 1)
	require.Contains(t, c.Errors[0].Error(), "failed to update config: persist failed")
}

func TestVerifyStartNewAgentCleansUpWhenProviderStartFails(t *testing.T) {
	previousInitAgentConfigWithCertsFunc := initAgentConfigWithCertsFunc
	previousNewAgentProviderFunc := newAgentProviderFunc
	previousState := configtypes.ActiveState.Load()
	previousAgents := agentpool.List()

	testState := newVerifyTestState(t)
	configtypes.ActiveState.Store(testState)
	agentpool.RemoveAll()

	t.Cleanup(func() {
		initAgentConfigWithCertsFunc = previousInitAgentConfigWithCertsFunc
		newAgentProviderFunc = previousNewAgentProviderFunc
		if previousState != nil {
			configtypes.ActiveState.Store(previousState)
		} else {
			configtypes.ActiveState = synk.Value[configtypes.State]{}
		}
		agentpool.RemoveAll()
		for _, agentInfo := range previousAgents {
			agentpool.Add(agentInfo.AgentConfig)
		}
	})

	const host = "10.0.0.9:8890"
	initAgentConfigWithCertsFunc = func(cfg *agent.AgentConfig, ctx context.Context, ca, crt, key []byte) error {
		cfg.Name = "start-fails"
		return nil
	}

	startErr := errors.New("start failed")
	fakeProvider := &verifyTestProvider{name: "agent.start-fails", startErr: startErr}
	newAgentProviderFunc = func(cfg *agent.AgentConfig) routing.Provider {
		require.Equal(t, host, cfg.Addr)
		return fakeProvider
	}

	nRoutes, cleanup, err := verifyStartNewAgent(t.Context(), host, agent.PEMPair{}, agent.PEMPair{}, agent.ContainerRuntimeDocker)
	require.Equal(t, 0, nRoutes)
	require.Nil(t, cleanup)
	require.ErrorContains(t, err, "failed to start routes: start failed")
	require.Equal(t, 1, fakeProvider.loadRoutesCalls)
	require.Equal(t, 1, fakeProvider.startCalls)
	require.False(t, agentpool.Has(&agent.AgentConfig{Addr: host}))
	require.Equal(t, 0, testState.NumProviders())

	fakeProvider = &verifyTestProvider{name: "agent.start-fails", numRoutes: 3}

	nRoutes, cleanup, err = verifyStartNewAgent(t.Context(), host, agent.PEMPair{}, agent.PEMPair{}, agent.ContainerRuntimeDocker)
	require.NoError(t, err)
	require.Equal(t, 3, nRoutes)
	require.NotNil(t, cleanup)
	require.True(t, agentpool.Has(&agent.AgentConfig{Addr: host}))
	require.Equal(t, 1, testState.NumProviders())
}

func TestVerifyStartNewAgentCleanupStopsStartedAgentRoutes(t *testing.T) {
	previousInitAgentConfigWithCertsFunc := initAgentConfigWithCertsFunc
	previousNewAgentProviderFunc := newAgentProviderFunc
	previousState := configtypes.ActiveState.Load()
	previousAgents := agentpool.List()

	testState := newVerifyTestState(t)
	configtypes.ActiveState.Store(testState)
	agentpool.RemoveAll()

	t.Cleanup(func() {
		initAgentConfigWithCertsFunc = previousInitAgentConfigWithCertsFunc
		newAgentProviderFunc = previousNewAgentProviderFunc
		if previousState != nil {
			configtypes.ActiveState.Store(previousState)
		} else {
			configtypes.ActiveState = synk.Value[configtypes.State]{}
		}
		agentpool.RemoveAll()
		for _, agentInfo := range previousAgents {
			agentpool.Add(agentInfo.AgentConfig)
		}
	})

	const host = "10.0.0.11:8890"
	initAgentConfigWithCertsFunc = func(cfg *agent.AgentConfig, ctx context.Context, ca, crt, key []byte) error {
		cfg.Name = "cleanup-started"
		return nil
	}

	fakeProvider := &verifyTestProvider{
		name:       "agent.cleanup-started",
		numRoutes:  2,
		started:    make(chan struct{}),
		cancelled:  make(chan struct{}),
		createTask: true,
	}
	newAgentProviderFunc = func(cfg *agent.AgentConfig) routing.Provider {
		require.Equal(t, host, cfg.Addr)
		return fakeProvider
	}

	nRoutes, cleanup, err := verifyStartNewAgent(t.Context(), host, agent.PEMPair{}, agent.PEMPair{}, agent.ContainerRuntimeDocker)
	require.NoError(t, err)
	require.Equal(t, 2, nRoutes)
	require.NotNil(t, cleanup)
	require.True(t, agentpool.Has(&agent.AgentConfig{Addr: host}))
	require.Equal(t, 1, testState.NumProviders())
	require.NotNil(t, fakeProvider.task)

	cleanupErr := errors.New("write failed")
	cleanup(cleanupErr)
	require.Eventually(t, func() bool {
		select {
		case <-fakeProvider.cancelled:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
	require.False(t, agentpool.Has(&agent.AgentConfig{Addr: host}))
	require.Equal(t, 0, testState.NumProviders())
	require.ErrorIs(t, context.Cause(fakeProvider.task.Context()), cleanupErr)
}

func TestVerifyStartNewAgentKeepsPartiallyStartedProvider(t *testing.T) {
	previousInitAgentConfigWithCertsFunc := initAgentConfigWithCertsFunc
	previousNewAgentProviderFunc := newAgentProviderFunc
	previousState := configtypes.ActiveState.Load()
	previousAgents := agentpool.List()

	testState := newVerifyTestState(t)
	configtypes.ActiveState.Store(testState)
	agentpool.RemoveAll()

	t.Cleanup(func() {
		initAgentConfigWithCertsFunc = previousInitAgentConfigWithCertsFunc
		newAgentProviderFunc = previousNewAgentProviderFunc
		if previousState != nil {
			configtypes.ActiveState.Store(previousState)
		} else {
			configtypes.ActiveState = synk.Value[configtypes.State]{}
		}
		agentpool.RemoveAll()
		for _, agentInfo := range previousAgents {
			agentpool.Add(agentInfo.AgentConfig)
		}
	})

	const host = "10.0.0.10:8890"
	initAgentConfigWithCertsFunc = func(cfg *agent.AgentConfig, ctx context.Context, ca, crt, key []byte) error {
		cfg.Name = "partial-start"
		return nil
	}

	startErr := errors.New("some routes failed")
	fakeProvider := &verifyTestProvider{name: "agent.partial-start", startErr: startErr, numRoutes: 2}
	newAgentProviderFunc = func(cfg *agent.AgentConfig) routing.Provider {
		require.Equal(t, host, cfg.Addr)
		return fakeProvider
	}

	nRoutes, cleanup, err := verifyStartNewAgent(t.Context(), host, agent.PEMPair{}, agent.PEMPair{}, agent.ContainerRuntimeDocker)
	require.Equal(t, 0, nRoutes)
	require.Nil(t, cleanup)
	require.ErrorContains(t, err, "failed to start routes: some routes failed")
	require.True(t, agentpool.Has(&agent.AgentConfig{Addr: host}))
	require.Equal(t, 1, testState.NumProviders())

	nRoutes, cleanup, err = verifyStartNewAgent(t.Context(), host, agent.PEMPair{}, agent.PEMPair{}, agent.ContainerRuntimeDocker)
	require.Equal(t, 0, nRoutes)
	require.Nil(t, cleanup)
	require.ErrorIs(t, err, errAgentAlreadyExists)
}

type verifyTestState struct {
	cfg       configtypes.Config
	task      *task.Task
	providers map[string]routing.Provider
}

func newVerifyTestState(t *testing.T) *verifyTestState {
	t.Helper()
	return &verifyTestState{
		cfg:       configtypes.DefaultConfig(),
		task:      task.GetTestTask(t),
		providers: make(map[string]routing.Provider),
	}
}

func (s *verifyTestState) InitFromFile(string) error      { return nil }
func (s *verifyTestState) Init([]byte) error              { return nil }
func (s *verifyTestState) Task() *task.Task               { return s.task }
func (s *verifyTestState) Context() context.Context       { return s.task.Context() }
func (s *verifyTestState) Value() *configtypes.Config     { return &s.cfg }
func (s *verifyTestState) Entrypoint() routing.Entrypoint { return nil }
func (s *verifyTestState) ShortLinkMatcher() configtypes.ShortLinkMatcher {
	return nil
}
func (s *verifyTestState) AutoCertProvider() server.CertProvider { return nil }
func (s *verifyTestState) LoadOrStoreProvider(key string, value routing.Provider) (routing.Provider, bool) {
	if provider, ok := s.providers[key]; ok {
		return provider, true
	}
	s.providers[key] = value
	return value, false
}
func (s *verifyTestState) DeleteProvider(key string) {
	delete(s.providers, key)
}
func (s *verifyTestState) IterProviders() iter.Seq2[string, routing.Provider] {
	return func(yield func(string, routing.Provider) bool) {
		for key, provider := range s.providers {
			if !yield(key, provider) {
				return
			}
		}
	}
}
func (s *verifyTestState) NumProviders() int     { return len(s.providers) }
func (s *verifyTestState) StartProviders() error { return nil }
func (s *verifyTestState) FlushTmpLog() error    { return nil }
func (s *verifyTestState) StartAPIServers()      {}
func (s *verifyTestState) StartMetrics()         {}

var _ configtypes.State = (*verifyTestState)(nil)

type verifyTestProvider struct {
	name            string
	loadRoutesCalls int
	startCalls      int
	startErr        error
	numRoutes       int
	createTask      bool
	task            *task.Task
	started         chan struct{}
	cancelled       chan struct{}
}

func (p *verifyTestProvider) Start(parent task.Parent) error {
	p.startCalls++
	if p.createTask {
		p.task = parent.Subtask("provider."+p.name, false)
		if p.started != nil {
			close(p.started)
		}
		if p.cancelled != nil {
			p.task.OnCancel("record_cancel", func() {
				close(p.cancelled)
			})
		}
	}
	return p.startErr
}
func (p *verifyTestProvider) LoadRoutes() error {
	p.loadRoutesCalls++
	return nil
}
func (p *verifyTestProvider) GetRoute(string) (routing.Route, bool) { return nil, false }
func (p *verifyTestProvider) IterRoutes(func(string, routing.Route) bool) {
}
func (p *verifyTestProvider) NumRoutes() int { return p.numRoutes }
func (p *verifyTestProvider) FindService(string, string) (routing.Route, bool) {
	return nil, false
}
func (p *verifyTestProvider) Statistics() routing.ProviderStats {
	return routing.ProviderStats{Type: routing.ProviderTypeAgent}
}
func (p *verifyTestProvider) GetType() routing.ProviderType { return routing.ProviderTypeAgent }
func (p *verifyTestProvider) ShortName() string             { return p.name }
func (p *verifyTestProvider) String() string                { return p.name }

var _ routing.Provider = (*verifyTestProvider)(nil)
