package rules

import (
	"testing"

	"github.com/rs/zerolog/log"
)

func TestErrorFormat(t *testing.T) {
	var rules Rules
	err := parseRules(`
- on: error 405
  do: error 405 error
- on: header too many args
  do: error 405 error
- name: missing do
  on: status 200
- on: header X-Header
  do: set invalid_command
- do: set resp_body "{{ .Request.Method {{ .Request.URL.Path }}"
`, &rules)
	log.Err(err).Msg("error")
}
