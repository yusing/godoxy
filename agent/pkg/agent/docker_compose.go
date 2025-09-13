package agent

import (
	"bytes"
	"text/template"

	_ "embed"
)

var (
	//go:embed templates/agent.compose.yml.tmpl
	agentComposeYAML         string
	agentComposeYAMLTemplate = template.Must(template.New("agent.compose.yml.tmpl").Parse(agentComposeYAML))
)

const (
	DockerImageProduction = "ghcr.io/yusing/godoxy-agent:latest"
	DockerImageNightly    = "ghcr.io/yusing/godoxy-agent:nightly"
)

func (c *AgentComposeConfig) Generate() (string, error) {
	buf := bytes.NewBuffer(make([]byte, 0, 1024))
	err := agentComposeYAMLTemplate.Execute(buf, c)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}
