package rules

import (
	"testing"

	"github.com/yusing/godoxy/internal/serialization"
	expect "github.com/yusing/goutils/testing"
)

func TestParseRule(t *testing.T) {
	test := []map[string]any{
		{
			"name": "test",
			"on":   "method POST",
			"do":   "error 403 Forbidden",
		},
		{
			"name": "auth",
			"on":   `basic_auth "username" "password" | basic_auth username2 "password2" | basic_auth "username3" "password3"`,
			"do":   "bypass",
		},
		{
			"name": "default",
			"do":   "require_basic_auth any_realm",
		},
	}

	var rules struct {
		Rules Rules
	}
	err := serialization.MapUnmarshalValidate(serialization.SerializedObject{"rules": test}, &rules)
	expect.NoError(t, err)
	expect.Equal(t, len(rules.Rules), len(test))
	expect.Equal(t, rules.Rules[0].Name, "test")
	expect.Equal(t, rules.Rules[0].On.String(), "method POST")
	expect.Equal(t, rules.Rules[0].Do.String(), "error 403 Forbidden")

	expect.Equal(t, rules.Rules[1].Name, "auth")
	expect.Equal(t, rules.Rules[1].On.String(), `basic_auth "username" "password" | basic_auth username2 "password2" | basic_auth "username3" "password3"`)
	expect.Equal(t, rules.Rules[1].Do.String(), "bypass")

	expect.Equal(t, rules.Rules[2].Name, "default")
	expect.Equal(t, rules.Rules[2].Do.String(), "require_basic_auth any_realm")
}

// TODO: real tests.
