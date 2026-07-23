package routing

import (
	gperr "github.com/yusing/goutils/errs"
)

type ProviderActivationHealth string

const (
	ProviderActivationReady    ProviderActivationHealth = "ready"
	ProviderActivationDegraded ProviderActivationHealth = "degraded"
	ProviderActivationFailed   ProviderActivationHealth = "failed"
)

type RouteActivationIssue struct {
	Route string      `json:"route"`
	Err   gperr.Error `json:"error"`
}

type ProviderActivation struct {
	Provider            string                 `json:"provider"`
	DesiredRoutes       int                    `json:"desired_routes"`
	AttemptedRoutes     int                    `json:"attempted_routes"`
	ActiveRoutes        int                    `json:"active_routes"`
	FailedRoutes        []RouteActivationIssue `json:"failed_routes,omitempty" extensions:"x-omitempty"`
	EventLoopReady      bool                   `json:"event_loop_ready"`
	InfrastructureError gperr.Error            `json:"infrastructure_error,omitempty" extensions:"x-omitempty"`
}

func (activation ProviderActivation) Health() ProviderActivationHealth {
	switch {
	case activation.InfrastructureError != nil && activation.ActiveRoutes == 0:
		return ProviderActivationFailed
	case activation.InfrastructureError != nil:
		return ProviderActivationDegraded
	case !activation.EventLoopReady:
		return ProviderActivationDegraded
	case activation.DesiredRoutes == 0:
		return ProviderActivationReady
	case activation.ActiveRoutes == activation.AttemptedRoutes && len(activation.FailedRoutes) == 0:
		return ProviderActivationReady
	case activation.ActiveRoutes > 0:
		return ProviderActivationDegraded
	default:
		return ProviderActivationFailed
	}
}

type ProviderActivationReport struct {
	Providers         []ProviderActivation `json:"providers"`
	ReadyProviders    int                  `json:"ready_providers"`
	DegradedProviders int                  `json:"degraded_providers"`
	FailedProviders   int                  `json:"failed_providers"`
	DesiredRoutes     int                  `json:"desired_routes"`
	AttemptedRoutes   int                  `json:"attempted_routes"`
	ActiveRoutes      int                  `json:"active_routes"`
	FailedRoutes      int                  `json:"failed_routes"`
}

func (report *ProviderActivationReport) Add(activation ProviderActivation) {
	report.Providers = append(report.Providers, activation)
	report.DesiredRoutes += activation.DesiredRoutes
	report.AttemptedRoutes += activation.AttemptedRoutes
	report.ActiveRoutes += activation.ActiveRoutes
	report.FailedRoutes += len(activation.FailedRoutes)

	switch activation.Health() {
	case ProviderActivationReady:
		report.ReadyProviders++
	case ProviderActivationDegraded:
		report.DegradedProviders++
	case ProviderActivationFailed:
		report.FailedProviders++
	}
}

// AllFailed reports whether activation attempted useful provider or route work
// and none of it became active. Ready empty providers do not count as failed
// attempts.
func (report ProviderActivationReport) AllFailed() bool {
	if report.ActiveRoutes > 0 {
		return false
	}
	return report.AttemptedRoutes > 0 || report.FailedProviders > 0
}
