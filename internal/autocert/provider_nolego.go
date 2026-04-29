package autocert

import "context"

func (p *Provider) obtainCertLocally(ctx context.Context) error {
	if p.cfg.Provider == ProviderLocal || p.cfg.Provider == ProviderPseudo {
		return nil
	}
	return obtainCertUsingBinary(ctx, p.cfg.CertPath)
}

func (p *Provider) certState() CertState {
	return CertStateValid
}

func (p *Provider) renew(ctx context.Context, mode RenewMode) (renewed bool, err error) {
	if mode == renewModeIfNeeded && p.certState() == CertStateValid {
		return false, nil
	}
	if err := p.ObtainCert(ctx); err != nil {
		return false, err
	}
	return true, nil
}
