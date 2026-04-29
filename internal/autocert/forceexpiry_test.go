package autocert

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestForceExpiryAllSkipsProviderWithoutRenewalWorker(t *testing.T) {
	for _, providerName := range []string{ProviderLocal, ProviderPseudo} {
		t.Run(providerName, func(t *testing.T) {
			provider := &Provider{
				cfg:            &Config{Provider: providerName},
				forceRenewalCh: make(chan struct{}, 1),
			}
			provider.forceRenewalDoneCh.Store(&emptyForceRenewalDoneCh)

			require.False(t, provider.ForceExpiryAll())
			require.Empty(t, provider.forceRenewalCh)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			require.False(t, provider.WaitRenewalDone(ctx))
		})
	}
}

func TestForceExpiryAllSkipsLocalMainAndForcesExtraProviders(t *testing.T) {
	extra := &Provider{
		forceRenewalCh: make(chan struct{}, 1),
	}
	extra.forceRenewalDoneCh.Store(&emptyForceRenewalDoneCh)

	provider := &Provider{
		cfg:            &Config{Provider: ProviderLocal},
		forceRenewalCh: make(chan struct{}, 1),
		extraProviders: []*Provider{extra},
	}
	provider.forceRenewalDoneCh.Store(&emptyForceRenewalDoneCh)

	require.True(t, provider.ForceExpiryAll())
	require.Empty(t, provider.forceRenewalCh)
	require.Len(t, extra.forceRenewalCh, 1)

	extra.finishForceRenewal(extra.forceRenewalDoneCh.Load())
	require.True(t, provider.WaitRenewalDone(context.Background()))
}

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

func TestFinishForceRenewalKeepsDoneSignalVisible(t *testing.T) {
	provider := &Provider{}
	provider.forceRenewalDoneCh.Store(&emptyForceRenewalDoneCh)

	done := provider.beginForceRenewal()
	require.NotNil(t, done)

	provider.finishForceRenewal(done)

	require.True(t, provider.WaitRenewalDone(context.Background()))
}

func TestBeginForceRenewalReplacesCompletedChannel(t *testing.T) {
	provider := &Provider{}
	provider.forceRenewalDoneCh.Store(&emptyForceRenewalDoneCh)

	done := provider.beginForceRenewal()
	require.NotNil(t, done)
	provider.finishForceRenewal(done)

	next := provider.beginForceRenewal()
	require.NotNil(t, next)
	require.NotSame(t, done, next)
}
