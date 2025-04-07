package metrics

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/yusing/go-proxy/internal/common"
)

type (
	RouteMetrics struct {
		HTTPReqTotal,
		HTTP2xx3xx,
		HTTP4xx,
		HTTP5xx *Counter
		HTTPReqElapsed *Gauge
	}
)

var rm RouteMetrics

const (
	routerNamespace     = "router"
	routerHTTPSubsystem = "http"

	serviceNamespace = "service"
)

func GetRouteMetrics() *RouteMetrics {
	return &rm
}

func (rm *RouteMetrics) UnregisterService(service string) {
	lbls := &HTTPRouteMetricLabels{Service: service}
	rm.HTTP2xx3xx.Delete(lbls)
	rm.HTTP4xx.Delete(lbls)
	rm.HTTP5xx.Delete(lbls)
	rm.HTTPReqElapsed.Delete(lbls)
}

func init() {
	if !common.PrometheusEnabled {
		return
	}
	initRouteMetrics()
}

func initRouteMetrics() {
	lbls := []string{"service", "method", "host", "visitor", "path"}
	partitionsHelp := ", partitioned by " + strings.Join(lbls, ", ")
	rm = RouteMetrics{
		HTTPReqTotal: NewCounter(prometheus.CounterOpts{
			Namespace: routerNamespace,
			Subsystem: routerHTTPSubsystem,
			Name:      "req_total",
			Help:      "How many requests processed in total",
		}),
		HTTP2xx3xx: NewCounter(prometheus.CounterOpts{
			Namespace: routerNamespace,
			Subsystem: routerHTTPSubsystem,
			Name:      "req_ok_count",
			Help:      "How many 2xx-3xx requests processed" + partitionsHelp,
		}, lbls...),
		HTTP4xx: NewCounter(prometheus.CounterOpts{
			Namespace: routerNamespace,
			Subsystem: routerHTTPSubsystem,
			Name:      "req_4xx_count",
			Help:      "How many 4xx requests processed" + partitionsHelp,
		}, lbls...),
		HTTP5xx: NewCounter(prometheus.CounterOpts{
			Namespace: routerNamespace,
			Subsystem: routerHTTPSubsystem,
			Name:      "req_5xx_count",
			Help:      "How many 5xx requests processed" + partitionsHelp,
		}, lbls...),
		HTTPReqElapsed: NewGauge(prometheus.GaugeOpts{
			Namespace: routerNamespace,
			Subsystem: routerHTTPSubsystem,
			Name:      "req_elapsed_ms",
			Help:      "How long it took to process the request and respond a status code" + partitionsHelp,
		}, lbls...),
	}
}
