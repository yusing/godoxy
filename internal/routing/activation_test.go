package routing

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	gperr "github.com/yusing/goutils/errs"
	strutils "github.com/yusing/goutils/strings"
)

type structuredProviderActivationError struct {
	Kind string `json:"kind"`
}

func (err structuredProviderActivationError) Error() string {
	return "structured provider activation error"
}

func (err structuredProviderActivationError) MarshalJSON() ([]byte, error) {
	type errorJSON structuredProviderActivationError
	return json.Marshal(errorJSON(err))
}

type futureProviderActivationError struct{}

func (futureProviderActivationError) Error() string {
	return "future provider activation error"
}

type malformedProviderActivationError struct {
	err error
}

func (err malformedProviderActivationError) Error() string {
	return "malformed provider activation error"
}

func (err malformedProviderActivationError) MarshalJSON() ([]byte, error) {
	return nil, err.err
}

func TestProviderActivationDefaultJSONPreservesInfrastructureErrorDiagnostics(t *testing.T) {
	t.Run("nil error is omitted", func(t *testing.T) {
		encoded, err := strutils.MarshalJSON(ProviderActivation{Provider: "empty"})
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "infrastructure_error")
	})

	t.Run("plain error remains identifiable and becomes readable JSON", func(t *testing.T) {
		sentinel := errors.New("provider infrastructure unavailable")
		activation := ProviderActivation{Provider: "docker", InfrastructureError: gperr.Wrap(sentinel)}

		encoded, err := strutils.MarshalJSON(activation)
		require.NoError(t, err)
		require.JSONEq(t, `{
			"provider":"docker",
			"desired_routes":0,
			"attempted_routes":0,
			"active_routes":0,
			"event_loop_ready":false,
			"infrastructure_error":"provider infrastructure unavailable"
		}`, string(encoded))
		require.ErrorIs(t, activation.InfrastructureError, sentinel)
	})

	t.Run("error-owned JSON formatting is preserved", func(t *testing.T) {
		activation := ProviderActivation{
			Provider: "future",
			InfrastructureError: gperr.Wrap(structuredProviderActivationError{
				Kind: "future-provider",
			}),
		}

		encoded, err := strutils.MarshalJSON(activation)
		require.NoError(t, err)
		var payload struct {
			InfrastructureError map[string]string `json:"infrastructure_error"`
		}
		require.NoError(t, json.Unmarshal(encoded, &payload))
		require.Equal(t, map[string]string{"kind": "future-provider"}, payload.InfrastructureError)
	})

	t.Run("unknown future error falls back to its readable form", func(t *testing.T) {
		encoded, err := strutils.MarshalJSON(ProviderActivation{
			Provider:            "future",
			InfrastructureError: gperr.Wrap(futureProviderActivationError{}),
		})
		require.NoError(t, err)
		require.Contains(t, string(encoded), `"infrastructure_error":"future provider activation error"`)
	})

	t.Run("malformed error serialization is returned to the caller", func(t *testing.T) {
		sentinel := errors.New("provider error serializer failed")
		_, err := strutils.MarshalJSON(ProviderActivation{
			Provider: "malformed",
			InfrastructureError: gperr.Wrap(malformedProviderActivationError{
				err: sentinel,
			}),
		})
		require.ErrorIs(t, err, sentinel)
	})

	t.Run("joined errors retain structure and identity", func(t *testing.T) {
		first := errors.New("provider unavailable")
		second := errors.New("activation canceled")
		activation := ProviderActivation{
			Provider:            "joined",
			InfrastructureError: gperr.Join(first, second),
		}

		encoded, err := strutils.MarshalJSON(activation)
		require.NoError(t, err)
		require.Contains(t, string(encoded), "provider unavailable")
		require.Contains(t, string(encoded), "activation canceled")
		require.ErrorIs(t, activation.InfrastructureError, first)
		require.ErrorIs(t, activation.InfrastructureError, second)
	})
}

func TestProviderActivationNeedsNoCustomJSONMarshaler(t *testing.T) {
	_, hasCustomMarshaler := any(ProviderActivation{}).(json.Marshaler)
	require.False(t, hasCustomMarshaler)

	encoded, err := strutils.MarshalJSON(ProviderActivation{
		Provider:            "docker",
		InfrastructureError: gperr.Wrap(errors.New("provider infrastructure unavailable")),
	})
	require.NoError(t, err)

	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &payload))
	require.JSONEq(t, `"provider infrastructure unavailable"`, string(payload["infrastructure_error"]))
}
