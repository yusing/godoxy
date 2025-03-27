package metricslogger

import (
	"net"
	"net/http"

	"github.com/yusing/go-proxy/internal/metrics"
)

type MetricsLogger struct {
	ServiceName string `json:"service_name"`
}

func NewMetricsLogger(serviceName string) *MetricsLogger {
	return &MetricsLogger{serviceName}
}

func (m *MetricsLogger) GetHandler(next http.Handler) http.HandlerFunc {
	return func(rw http.ResponseWriter, req *http.Request) {
		m.ServeHTTP(rw, req, next.ServeHTTP)
	}
}

func (m *MetricsLogger) ServeHTTP(rw http.ResponseWriter, req *http.Request, next http.HandlerFunc) {
	visitorIP, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		visitorIP = req.RemoteAddr
	}

	// req.RemoteAddr had been modified by middleware (if any)
	lbls := &metrics.HTTPRouteMetricLabels{
		Service: m.ServiceName,
		Method:  req.Method,
		Host:    req.Host,
		Visitor: visitorIP,
		Path:    req.URL.Path,
	}

	next.ServeHTTP(newHTTPMetricLogger(rw, lbls), req)
}

func (m *MetricsLogger) ResetMetrics() {
	metrics.GetRouteMetrics().UnregisterService(m.ServiceName)
}
