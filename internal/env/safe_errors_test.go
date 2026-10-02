package env

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"testing"
)

func TestInvalidInputDoesNotDiscloseValue(t *testing.T) {
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previous) })
	oldPrefixes := envPrefixes
	SetPrefixes("GODOXY_")
	t.Cleanup(func() { envPrefixes = oldPrefixes })
	for _, tc := range []struct {
		name, value string
		read        func()
	}{
		{"boolean", "private-invalid-input", func() { GetEnvBool("SAFE_ERROR_TEST", false) }},
		{"duration", "private-invalid-input", func() { GetEnvDuation("SAFE_ERROR_TEST", 0) }},
		{"address", "private-invalid-input", func() { GetAddrEnv("SAFE_ERROR_TEST", "", "http") }},
		{"port", "localhost:private-invalid-input", func() { GetAddrEnv("SAFE_ERROR_TEST", "", "http") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output.Reset()
			t.Setenv("GODOXY_SAFE_ERROR_TEST", tc.value)
			defer func() {
				value := recover()
				if value == nil {
					t.Fatal("invalid input accepted")
				}
				if strings.Contains(fmt.Sprint(value), "private-invalid-input") || strings.Contains(output.String(), "private-invalid-input") {
					t.Fatal("invalid input was disclosed")
				}
			}()
			tc.read()
		})
	}
}
