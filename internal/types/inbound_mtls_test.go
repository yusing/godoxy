package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInboundMTLSProfileValidate(t *testing.T) {
	t.Run("requires at least one trust source", func(t *testing.T) {
		var profile InboundMTLSProfile
		err := profile.Validate()
		require.Error(t, err)
		require.ErrorContains(t, err, "trust source")
	})

	t.Run("system CA only", func(t *testing.T) {
		profile := InboundMTLSProfile{UseSystemCAs: true}
		require.NoError(t, profile.Validate())
	})

	t.Run("CA file only", func(t *testing.T) {
		profile := InboundMTLSProfile{CAFiles: []string{"/tmp/ca.pem"}}
		require.NoError(t, profile.Validate())
	})

	t.Run("system CA and CA files", func(t *testing.T) {
		profile := InboundMTLSProfile{
			UseSystemCAs: true,
			CAFiles:      []string{"/tmp/ca.pem"},
		}
		require.NoError(t, profile.Validate())
	})
}
