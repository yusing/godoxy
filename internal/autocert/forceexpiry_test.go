package autocert

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestForceExpiryAllStillForcesExtraProvidersWhenMainAlreadyRunning(t *testing.T) {
	mainDone := make(chan struct{})
	extra := &Provider{
		forceRenewalCh: make(chan struct{}, 1),
	}
	extra.forceRenewalDoneCh.Store(&emptyForceRenewalDoneCh)

	provider := &Provider{
		forceRenewalCh: make(chan struct{}, 1),
		extraProviders: []*Provider{extra},
	}
	provider.forceRenewalDoneCh.Store(&mainDone)

	ok := provider.ForceExpiryAll()
	require.True(t, ok)
	require.Empty(t, provider.forceRenewalCh)
	require.Len(t, extra.forceRenewalCh, 1)
}
