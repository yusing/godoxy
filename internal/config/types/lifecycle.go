package config

import (
	"errors"

	"github.com/yusing/godoxy/internal/routing"
	gperr "github.com/yusing/goutils/errs"
)

type ActivationHealth string

const (
	ActivationHealthy  ActivationHealth = "healthy"
	ActivationDegraded ActivationHealth = "degraded"
	ActivationFailed   ActivationHealth = "failed"
)

type RuntimeStatus string

const (
	RuntimePreparing  RuntimeStatus = "preparing"
	RuntimeActivating RuntimeStatus = "activating"
	RuntimeHealthy    RuntimeStatus = "healthy"
	RuntimeDegraded   RuntimeStatus = "degraded"
	RuntimeFailed     RuntimeStatus = "failed"
	RuntimeStopping   RuntimeStatus = "stopping"
)

type IssueSeverity string

const (
	IssueRejecting IssueSeverity = "rejecting"
	IssueDegraded  IssueSeverity = "degraded"
	IssueFailed    IssueSeverity = "failed"
)

// IsFailure applies the lifecycle's fail-closed severity policy. Unknown
// future values are failures until their semantics are explicitly defined.
func (severity IssueSeverity) IsFailure() bool {
	return severity != IssueDegraded
}

type ActivationIssue struct {
	Component string        `json:"component"`
	Subject   string        `json:"subject,omitempty"`
	Severity  IssueSeverity `json:"severity"`
	Err       gperr.Error   `json:"error"`
}

type ComponentActivation struct {
	Configured bool        `json:"configured"`
	Required   bool        `json:"required"`
	Ready      bool        `json:"ready"`
	Err        gperr.Error `json:"error,omitempty" extensions:"x-omitempty"`
}

type APIActivationReport struct {
	Main  ComponentActivation `json:"main"`
	Local ComponentActivation `json:"local"`
}

type ActivationReport struct {
	Providers routing.ProviderActivationReport `json:"providers"`
	API       APIActivationReport              `json:"api"`
	Metrics   ComponentActivation              `json:"metrics"`
}

type ReloadResult struct {
	Committed bool              `json:"committed"`
	Health    ActivationHealth  `json:"health"`
	Issues    []ActivationIssue `json:"issues,omitempty"`
	ActivationReport
}

// IssueError preserves all lifecycle error identities in one projection for
// logs, notifications, and process policy.
func (result ReloadResult) IssueError() error {
	errs := make([]error, 0, len(result.Issues))
	for _, issue := range result.Issues {
		if issue.Err != nil {
			errs = append(errs, issue.Err)
		}
	}
	return errors.Join(errs...)
}

type RuntimeSnapshot struct {
	Status     RuntimeStatus    `json:"status"`
	Health     ActivationHealth `json:"health,omitempty" extensions:"x-omitempty"`
	Activation ActivationReport `json:"activation"`
}
