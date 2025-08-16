package provider

type Type string //	@name	ProviderType

const (
	ProviderTypeDocker Type = "docker"
	ProviderTypeFile   Type = "file"
	ProviderTypeAgent  Type = "agent"
)
