package config

import (
	"os"
	"path"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/assert"
	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/internal/common"
	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/route/provider"
	"github.com/yusing/go-proxy/internal/utils"
	. "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestFileProviderValidate(t *testing.T) {
	tests := []struct {
		name                  string
		filenames             []string
		init, cleanup         func(filepath string) error
		expectedErrorContains string
	}{
		{
			name:                  "file not exists",
			filenames:             []string{"not_exists.yaml"},
			expectedErrorContains: "config_file_exists",
		},
		{
			name:                  "file is a directory",
			filenames:             []string{"testdata"},
			expectedErrorContains: "config_file_exists",
		},
		{
			name:                  "same file exists multiple times",
			filenames:             []string{"test.yml", "test.yml"},
			expectedErrorContains: "unique",
		},
		{
			name:      "file ok",
			filenames: []string{"routes.yaml"},
			init: func(filepath string) error {
				os.MkdirAll(path.Dir(filepath), 0755)
				_, err := os.Create(filepath)
				return err
			},
			cleanup: func(filepath string) error {
				return os.RemoveAll(path.Dir(filepath))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			if tt.init != nil {
				for _, filename := range tt.filenames {
					filepath := path.Join(common.ConfigDir, filename)
					assert.NoError(t, tt.init(filepath))
				}
			}
			err := utils.UnmarshalValidateYAML(Must(yaml.Marshal(map[string]any{
				"providers": map[string]any{
					"include": tt.filenames,
				},
			})), cfg)
			if tt.cleanup != nil {
				for _, filename := range tt.filenames {
					filepath := path.Join(common.ConfigDir, filename)
					assert.NoError(t, tt.cleanup(filepath))
				}
			}
			if tt.expectedErrorContains != "" {
				assert.ErrorContains(t, err, tt.expectedErrorContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLoadRouteProviders(t *testing.T) {
	tests := []struct {
		name          string
		providers     *config.Providers
		expectedError bool
	}{
		{
			name: "duplicate file provider",
			providers: &config.Providers{
				Files: []string{"routes.yaml", "routes.yaml"},
			},
			expectedError: true,
		},
		{
			name: "duplicate docker provider",
			providers: &config.Providers{
				Docker: map[string]string{
					"docker1": "unix:///var/run/docker.sock",
					"docker2": "unix:///var/run/docker.sock",
				},
			},
			expectedError: true,
		},
		{
			name: "docker provider with different hosts",
			providers: &config.Providers{
				Docker: map[string]string{
					"docker1": "unix:///var/run/docker1.sock",
					"docker2": "unix:///var/run/docker2.sock",
				},
			},
			expectedError: false,
		},
		{
			name: "duplicate agent addresses",
			providers: &config.Providers{
				Agents: []*agent.AgentConfig{
					{Addr: "192.168.1.100:8080"},
					{Addr: "192.168.1.100:8080"},
				},
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := utils.Validate(tt.providers)
			if tt.expectedError {
				assert.ErrorContains(t, err, "unique")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProviderNameUniqueness(t *testing.T) {
	file := provider.NewFileProvider("routes.yaml")
	docker := provider.NewDockerProvider("routes", "unix:///var/run/docker.sock")
	agent := provider.NewAgentProvider(agent.TestAgentConfig("routes", "192.168.1.100:8080"))

	assert.True(t, file.String() != docker.String())
	assert.True(t, file.String() != agent.String())
	assert.True(t, docker.String() != agent.String())
}

func TestFileProviderNameFromFilename(t *testing.T) {
	tests := []struct {
		filename     string
		expectedName string
	}{
		{"routes.yaml", "routes"},
		{"service.yml", "service"},
		{"complex-name.yaml", "complex-name"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			p := provider.NewFileProvider(tt.filename)
			assert.Equal(t, tt.expectedName, p.ShortName())
		})
	}
}

func TestDockerProviderString(t *testing.T) {
	tests := []struct {
		name       string
		dockerHost string
		expected   string
	}{
		{"docker1", "unix:///var/run/docker.sock", "docker@docker1"},
		{"host2", "tcp://192.168.1.100:2375", "docker@host2"},
		{"explicit!", "unix:///var/run/docker.sock", "docker@explicit!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := provider.NewDockerProvider(tt.name, tt.dockerHost)
			assert.Equal(t, tt.expected, p.String())
		})
	}
}

func TestExplicitOnlyProvider(t *testing.T) {
	tests := []struct {
		name         string
		expectedFlag bool
	}{
		{"docker", false},
		{"explicit!", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := provider.NewDockerProvider(tt.name, "unix:///var/run/docker.sock")
			assert.Equal(t, tt.expectedFlag, p.IsExplicitOnly())
		})
	}
}
