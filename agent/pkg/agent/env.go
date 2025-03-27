package agent

type (
	AgentEnvConfig struct {
		Name    string
		Port    int
		CACert  string
		SSLCert string
	}
	AgentComposeConfig struct {
		Image string
		*AgentEnvConfig
	}
	Generator interface {
		Generate() (string, error)
	}
)
