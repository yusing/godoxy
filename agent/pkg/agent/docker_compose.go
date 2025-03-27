package agent

import (
	"bytes"
	"text/template"

	_ "embed"
)

var (
	//go:embed templates/agent.compose.yml
	agentComposeYAML         string
	agentComposeYAMLTemplate = template.Must(template.New("agent.compose.yml").Parse(agentComposeYAML))
)

const (
	DockerImageProduction = "ghcr.io/yusing/godoxy-agent:latest"
	DockerImageNightly    = "ghcr.io/yusing/godoxy-agent:nightly"
)

func (c *AgentComposeConfig) Generate() (string, error) {
	buf := bytes.NewBuffer(make([]byte, 0, 1024))
	if err := agentComposeYAMLTemplate.Execute(buf, c); err != nil {
		return "", err
	}
	return buf.String(), nil
}
