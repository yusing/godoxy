package types

type ProviderType string

const (
	ProviderTypeDocker ProviderType = "docker"
	ProviderTypeFile   ProviderType = "file"
	ProviderTypeAgent  ProviderType = "agent"
)
