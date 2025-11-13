package provider

import (
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/moby/moby/api/types/container"
	"github.com/yusing/godoxy/internal/docker"
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
			Ports: []container.PortSummary{
				{Type: "tcp", PrivatePort: 1234, PublicPort: 1234},
			},
		}, "/var/run/docker.sock"),
	)
	expect.NoError(t, err)
	expect.True(t, routes.Contains("app"))
	expect.True(t, routes.Contains("app1"))
}
