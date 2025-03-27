package metricslogger

import (
	"net/http"
	"time"

	"github.com/yusing/go-proxy/internal/metrics"
)

type httpMetricLogger struct {
	http.ResponseWriter
	timestamp time.Time
	labels    *metrics.HTTPRouteMetricLabels
}

// WriteHeader implements http.ResponseWriter.
func (l *httpMetricLogger) WriteHeader(status int) {
	l.ResponseWriter.WriteHeader(status)
	duration := time.Since(l.timestamp)
	go func() {
		m := metrics.GetRouteMetrics()
		m.HTTPReqTotal.Inc()
		m.HTTPReqElapsed.With(l.labels).Set(float64(duration.Milliseconds()))

		// ignore 1xx
		switch {
		case status >= 500:
			m.HTTP5xx.With(l.labels).Inc()
		case status >= 400:
			m.HTTP4xx.With(l.labels).Inc()
		case status >= 200:
			m.HTTP2xx3xx.With(l.labels).Inc()
		}
	}()
}

func (l *httpMetricLogger) Unwrap() http.ResponseWriter {
	return l.ResponseWriter
}

func newHTTPMetricLogger(w http.ResponseWriter, labels *metrics.HTTPRouteMetricLabels) *httpMetricLogger {
	return &httpMetricLogger{
		ResponseWriter: w,
		timestamp:      time.Now(),
		labels:         labels,
	}
}
