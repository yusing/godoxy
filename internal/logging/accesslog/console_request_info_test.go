package accesslog

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestConsoleRequestInfo(t *testing.T) {
	for _, annotate := range []bool{false, true} {
		name := "without annotations"
		if annotate {
			name = "with annotations"
		}
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "https://app.example.com/", nil)
			r.RemoteAddr = "127.0.0.1:40000"
			r = WithRequestInfo(r)
			if annotate {
				r.RemoteAddr = "203.0.113.7:40000"
				RecordDenied(r, "CIDRWhitelist", "IP not allowed")
				RecordDenied(r, "RateLimit", "rate limit exceeded")
			}

			var output bytes.Buffer
			logger := zerolog.New(zerolog.ConsoleWriter{Out: &output, NoColor: true})
			cfg := DefaultRequestLoggerConfig()
			formatter := ConsoleFormatter{cfg: &cfg.Fields}
			formatter.LogRequestZeroLog(&logger, r, &http.Response{StatusCode: http.StatusForbidden})
			line := output.String()
			for _, field := range []string{"peer_ip=", "denied_by=", "deny_reason="} {
				if strings.Contains(line, field) != annotate {
					t.Errorf("annotation %q present=%v, want %v: %s", field, !annotate, annotate, line)
				}
			}
			if annotate && (!strings.Contains(line, "peer_ip=127.0.0.1") ||
				!strings.Contains(line, "denied_by=CIDRWhitelist") ||
				!strings.Contains(line, `deny_reason="IP not allowed"`) ||
				strings.Contains(line, "RateLimit")) {
				t.Errorf("incorrect console denial metadata: %s", line)
			}
		})
	}
}
