package docker

import (
	"os"
	"os/exec"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/types"
)

func TestPrefixedDockerHostIsLocal(t *testing.T) {
	if os.Getenv("GODOXY_DOCKER_HOST_TEST_CHILD") == "1" {
		require.Equal(t, "tcp://socket-proxy:2375", EnvDockerHost)
		c := FromDocker(t.Context(), &container.Summary{
			Names: []string{"app"}, State: "running",
			NetworkSettings: &container.NetworkSettingsSummary{
				Networks: map[string]*network.EndpointSettings{"bridge": {IPAddress: "172.17.0.2"}},
			},
		}, types.DockerProviderConfig{URL: EnvDockerHost})
		require.Equal(t, "127.0.0.1", c.PublicHostname)
		require.Equal(t, "172.17.0.2", c.PrivateHostname)
		return
	}
	for _, prefix := range []string{"GODOXY_", "GOPROXY_"} {
		t.Run(prefix, func(t *testing.T) {
			t.Setenv("GODOXY_DOCKER_HOST", "")
			t.Setenv("GOPROXY_DOCKER_HOST", "")
			t.Setenv("DOCKER_HOST", "tcp://lower-priority:2375")
			t.Setenv(prefix+"DOCKER_HOST", "tcp://socket-proxy:2375")
			t.Setenv("GODOXY_DOCKER_HOST_TEST_CHILD", "1")
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestPrefixedDockerHostIsLocal$")
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", output)
		})
	}
}
