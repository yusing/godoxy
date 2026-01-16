package provider

import (
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/goccy/go-yaml"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/types"
	expect "github.com/yusing/goutils/testing"

	_ "embed"
)

//go:embed docker_labels.yaml
var testDockerLabelsYAML []byte

func TestParseDockerLabels(t *testing.T) {
	var provider DockerProvider

	labels := make(map[string]string)
	expect.NoError(t, yaml.Unmarshal(testDockerLabelsYAML, &labels))

	routes, err := provider.routesFromContainerLabels(
		docker.FromDocker(&container.Summary{
			Names:  []string{"container"},
			Labels: labels,
			State:  "running",
			Ports: []container.Port{
				{Type: "tcp", PrivatePort: 1234, PublicPort: 1234},
			},
		}, types.DockerProviderConfig{URL: "unix:///var/run/docker.sock"}),
	)
	expect.NoError(t, err)
	expect.True(t, routes.Contains("app"))
	expect.True(t, routes.Contains("app1"))
}
