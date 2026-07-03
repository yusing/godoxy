package routing

type ProviderType string //	@name	ProviderType

const (
	ProviderTypeDocker ProviderType = "docker"
	ProviderTypeFile   ProviderType = "file"
	ProviderTypeAgent  ProviderType = "agent"
)
