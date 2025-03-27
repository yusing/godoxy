package metrics

import "github.com/prometheus/client_golang/prometheus"

type (
	HTTPRouteMetricLabels struct {
		Service, Method, Host, Visitor, Path string
	}
	StreamRouteMetricLabels struct {
		Service, Visitor string
	}
)

func (lbl *HTTPRouteMetricLabels) toPromLabels() prometheus.Labels {
	return prometheus.Labels{
		"service": lbl.Service,
		"method":  lbl.Method,
		"host":    lbl.Host,
		"visitor": lbl.Visitor,
		"path":    lbl.Path,
	}
}

func (lbl *StreamRouteMetricLabels) toPromLabels() prometheus.Labels {
	return prometheus.Labels{
		"service": lbl.Service,
		"visitor": lbl.Visitor,
	}
}
