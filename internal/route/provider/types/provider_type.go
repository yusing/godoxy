package provider

type Type string

const (
	ProviderTypeDocker Type = "docker"
	ProviderTypeFile   Type = "file"
	ProviderTypeAgent  Type = "agent"
)
