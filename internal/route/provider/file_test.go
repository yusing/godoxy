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
