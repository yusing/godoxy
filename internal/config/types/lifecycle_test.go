package config

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	gperr "github.com/yusing/goutils/errs"
	strutils "github.com/yusing/goutils/strings"
)

func TestLifecycleErrorsNeedNoCustomJSONMarshalers(t *testing.T) {
	_, issueMarshalsItself := any(ActivationIssue{}).(json.Marshaler)
	_, componentMarshalsItself := any(ComponentActivation{}).(json.Marshaler)
	require.False(t, issueMarshalsItself)
	require.False(t, componentMarshalsItself)

	sentinel := errors.New("activation failed")
	result := ReloadResult{
		Committed: true,
		Health:    ActivationDegraded,
		Issues: []ActivationIssue{{
			Component: "provider",
			Severity:  IssueDegraded,
			Err:       gperr.Wrap(sentinel),
		}},
		ActivationReport: ActivationReport{
			Metrics: ComponentActivation{
				Configured: true,
				Err:        gperr.Wrap(sentinel),
			},
		},
	}

	encoded, err := strutils.MarshalJSON(result)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"error":"activation failed"`)
	require.ErrorIs(t, result.Issues[0].Err, sentinel)
	require.ErrorIs(t, result.Metrics.Err, sentinel)

	encoded, err = strutils.MarshalJSON(ComponentActivation{})
	require.NoError(t, err)
	require.NotContains(t, string(encoded), `"error"`)
}

func TestIssueSeverityFailsClosed(t *testing.T) {
	tests := []struct {
		severity IssueSeverity
		failure  bool
	}{
		{severity: IssueDegraded, failure: false},
		{severity: IssueRejecting, failure: true},
		{severity: IssueFailed, failure: true},
		{severity: IssueSeverity("future"), failure: true},
		{severity: IssueSeverity(""), failure: true},
	}
	for _, test := range tests {
		require.Equal(t, test.failure, test.severity.IsFailure(), string(test.severity))
	}
}

func TestReloadResultIssueErrorPreservesIdentity(t *testing.T) {
	require.NoError(t, (ReloadResult{}).IssueError())

	first := errors.New("same message")
	unrelated := errors.New("same message")
	second := errors.New("future component failure")
	result := ReloadResult{Issues: []ActivationIssue{
		{Component: "first", Severity: IssueDegraded, Err: gperr.Wrap(first)},
		{Component: "empty", Severity: IssueDegraded},
		{Component: "future", Severity: IssueSeverity("future"), Err: gperr.Wrap(second)},
	}}

	err := result.IssueError()
	require.ErrorIs(t, err, first)
	require.ErrorIs(t, err, second)
	require.NotErrorIs(t, err, unrelated)
}
