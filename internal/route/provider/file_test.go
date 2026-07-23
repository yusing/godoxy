package provider

import (
	"testing"

	_ "embed"

	"github.com/stretchr/testify/require"
)

//go:embed fixtures/all_fields.yaml
var testAllFieldsYAML []byte

func TestFile(t *testing.T) {
	_, err := validate(testAllFieldsYAML)
	require.NoError(t, err)
}

func TestValidateIdlewatcherUsesRouteProxmoxBinding(t *testing.T) {
	data := []byte(`app:
  host: example.com
  proxmox:
    node: pve
    vmid: 119
  idlewatcher:
    idle_timeout: 30m
`)

	require.NoError(t, Validate(t.Context(), data))
}

func TestValidateIdlewatcherRequiresBoundProvider(t *testing.T) {
	data := []byte(`app:
  host: example.com
  idlewatcher:
    idle_timeout: 30m
`)

	err := Validate(t.Context(), data)
	require.ErrorContains(t, err, "missing idlewatcher provider config")
}
