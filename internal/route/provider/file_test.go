package provider

import (
	"testing"

	_ "embed"

	. "github.com/yusing/godoxy/internal/utils/testing"
)

//go:embed all_fields.yaml
var testAllFieldsYAML []byte

func TestFile(t *testing.T) {
	_, err := validate(testAllFieldsYAML)
	ExpectNoError(t, err)
}
