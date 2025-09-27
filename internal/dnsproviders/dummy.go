package dnsproviders

type (
	DummyConfig   struct{}
	DummyProvider struct{}
)

func NewDummyDefaultConfig() *DummyConfig {
	return &DummyConfig{}
}

func NewDummyDNSProviderConfig(*DummyConfig) (*DummyProvider, error) {
	return &DummyProvider{}, nil
}

func (DummyProvider) Present(domain, token, keyAuth string) error {
	return nil
}

func (DummyProvider) CleanUp(domain, token, keyAuth string) error {
	return nil
}
