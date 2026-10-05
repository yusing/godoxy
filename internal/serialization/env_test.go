package serialization

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/require"
)

func TestEnvironmentInterpolation(t *testing.T) {
	t.Setenv("GODOXY_INTERPOLATION_VALUE", "a: b # comment\n\"quotes\" and 'apostrophes' \\ ${LITERAL}")
	t.Setenv("INTERPOLATION_VALUE", "lower priority")
	value := "a: b # comment\n\"quotes\" and 'apostrophes' \\ ${LITERAL}"
	for _, input := range []string{
		"value: prefix-${INTERPOLATION_VALUE}-suffix",
		`value: "prefix-${INTERPOLATION_VALUE}-suffix"`,
		`value: 'prefix-${INTERPOLATION_VALUE}-suffix'`,
		`{value: prefix-${INTERPOLATION_VALUE}-suffix}`,
		"value: |-\n  prefix-${INTERPOLATION_VALUE}-suffix",
	} {
		t.Run(input, func(t *testing.T) {
			var result struct{ Value string }
			require.NoError(t, UnmarshalValidate([]byte(input), &result, yaml.Unmarshal))
			require.Equal(t, "prefix-"+value+"-suffix", result.Value)
		})
	}
	t.Setenv("GODOXY_INTERPOLATION_VALUE", "true")
	var result map[string]any
	require.NoError(t, UnmarshalValidate([]byte("value: ${INTERPOLATION_VALUE}\nlist: [${INTERPOLATION_VALUE}, '${INTERPOLATION_VALUE}']\n# ${UNSET_COMMENT}"), &result, yaml.Unmarshal))
	require.Equal(t, "true", result["value"])
	require.Equal(t, []any{"true", "true"}, result["list"])

	t.Setenv("GODOXY_INTERPOLATION_VALUE", "")
	t.Setenv("INTERPOLATION_VALUE", "")
	var empty struct{ Value string }
	require.NoError(t, UnmarshalValidate([]byte("value: ${INTERPOLATION_VALUE}"), &empty, yaml.Unmarshal))
	require.Empty(t, empty.Value)
	require.ErrorContains(t, UnmarshalValidate([]byte("value: ${GODOXY_MISSING_INTERPOLATION_TEST}"), &result, yaml.Unmarshal), "is not set")
	t.Setenv("GODOXY_INTERPOLATION_KEY", "value")
	require.ErrorContains(t, UnmarshalValidate([]byte("value: one\n${INTERPOLATION_KEY}: two"), &result, yaml.Unmarshal), "duplicate mapping key")
}

func TestBindingEnvironmentInterpolation(t *testing.T) {
	t.Setenv("GODOXY_BINDING_VALUE", "quoted \"value\"\nnext: line")
	for _, tc := range []struct {
		name, input string
		bind        func(*http.Request, any) error
	}{
		{"json", `{"value":"prefix-${BINDING_VALUE}"}`, GinJSONBinding{}.Bind},
		{"yaml", `value: prefix-${BINDING_VALUE}`, GinYAMLBinding{}.Bind},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var result struct{ Value string }
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.input))
			require.NoError(t, tc.bind(req, &result))
			require.Equal(t, "prefix-quoted \"value\"\nnext: line", result.Value)
		})
	}
}

func TestEnvironmentInterpolationAliasesExpandOnce(t *testing.T) {
	t.Setenv("GODOXY_ALIAS_VALUE", "${UNSET_ALIAS_LITERAL}")
	for _, input := range []string{
		"first: &shared\n  value: ${ALIAS_VALUE}\nsecond: *shared",
		"first: &shared [${ALIAS_VALUE}]\nsecond: *shared",
	} {
		t.Run(input, func(t *testing.T) {
			var result map[string]any
			require.NoError(t, UnmarshalValidate([]byte(input), &result, yaml.Unmarshal))
			require.Equal(t, result["first"], result["second"])
			switch first := result["first"].(type) {
			case map[string]any:
				require.Equal(t, "${UNSET_ALIAS_LITERAL}", first["value"])
			case []any:
				require.Equal(t, []any{"${UNSET_ALIAS_LITERAL}"}, first)
			default:
				t.Fatalf("unexpected alias value: %T", first)
			}
		})
	}
}
