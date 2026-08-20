package dnsproviders

import "context"

type (
	DummyConfig   map[string]any
	DummyProvider struct{}
)

func NewDummyDefaultConfig() *DummyConfig {
	return &DummyConfig{}
}

func NewDummyDNSProviderConfig(*DummyConfig) (*DummyProvider, error) {
	return &DummyProvider{}, nil
}

func (DummyProvider) Present(ctx context.Context, domain, token, keyAuth string) error {
	return nil
}

func (DummyProvider) CleanUp(ctx context.Context, domain, token, keyAuth string) error {
	return nil
}
