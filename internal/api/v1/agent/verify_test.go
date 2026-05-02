package agentapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/agent/pkg/agent"
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

	previousVerifyNewAgentFunc := verifyNewAgentFunc
	previousPersistAgentHostToConfigFunc := persistAgentHostToConfigFunc
	previousSuppressNextConfigReloadFunc := suppressNextConfigReloadFunc
	previousClearConfigReloadSuppressionFunc := clearConfigReloadSuppressionFunc
	previousWriteAgentCertZipFunc := writeAgentCertZipFunc
	previousListAgentsFunc := listAgentsFunc
	t.Cleanup(func() {
		verifyNewAgentFunc = previousVerifyNewAgentFunc
		persistAgentHostToConfigFunc = previousPersistAgentHostToConfigFunc
		suppressNextConfigReloadFunc = previousSuppressNextConfigReloadFunc
		clearConfigReloadSuppressionFunc = previousClearConfigReloadSuppressionFunc
		writeAgentCertZipFunc = previousWriteAgentCertZipFunc
		listAgentsFunc = previousListAgentsFunc
	})

	verifyNewAgentCalls := 0
	verifyNewAgentFunc = func(ctx context.Context, host string, ca agent.PEMPair, client agent.PEMPair, containerRuntime agent.ContainerRuntime) (int, error) {
		verifyNewAgentCalls++
		require.Equal(t, "10.0.0.1:8890", host)
		require.Equal(t, []byte("ca-cert"), ca.Cert)
		require.Equal(t, []byte("client-cert"), client.Cert)
		require.Equal(t, []byte("client-key"), client.Key)
		require.Equal(t, agent.ContainerRuntimeDocker, containerRuntime)
		return 2, nil
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

	previousVerifyNewAgentFunc := verifyNewAgentFunc
	previousPersistAgentHostToConfigFunc := persistAgentHostToConfigFunc
	previousSuppressNextConfigReloadFunc := suppressNextConfigReloadFunc
	previousClearConfigReloadSuppressionFunc := clearConfigReloadSuppressionFunc
	previousWriteAgentCertZipFunc := writeAgentCertZipFunc
	previousListAgentsFunc := listAgentsFunc
	t.Cleanup(func() {
		verifyNewAgentFunc = previousVerifyNewAgentFunc
		persistAgentHostToConfigFunc = previousPersistAgentHostToConfigFunc
		suppressNextConfigReloadFunc = previousSuppressNextConfigReloadFunc
		clearConfigReloadSuppressionFunc = previousClearConfigReloadSuppressionFunc
		writeAgentCertZipFunc = previousWriteAgentCertZipFunc
		listAgentsFunc = previousListAgentsFunc
	})

	verifyNewAgentCalls := 0
	verifyNewAgentFunc = func(ctx context.Context, host string, ca agent.PEMPair, client agent.PEMPair, containerRuntime agent.ContainerRuntime) (int, error) {
		verifyNewAgentCalls++
		require.Equal(t, "10.0.0.1:8890", host)
		return 2, nil
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
	require.Len(t, c.Errors, 1)
	require.Contains(t, c.Errors[0].Error(), "failed to update config: persist failed")
}
