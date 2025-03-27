package accesslog_test

import (
	"testing"

	. "github.com/yusing/go-proxy/internal/net/gphttp/accesslog"
	. "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestParseRetention(t *testing.T) {
	tests := []struct {
		input     string
		expected  *Retention
		shouldErr bool
	}{
		{"30 days", &Retention{Days: 30}, false},
		{"2 weeks", &Retention{Days: 14}, false},
		{"last 5", &Retention{Last: 5}, false},
		{"invalid input", &Retention{}, true},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			r := &Retention{}
			err := r.Parse(test.input)
			if !test.shouldErr {
				ExpectNoError(t, err)
			} else {
				ExpectDeepEqual(t, r, test.expected)
			}
		})
	}
}
