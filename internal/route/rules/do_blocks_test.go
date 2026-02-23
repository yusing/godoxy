package rules

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	httputils "github.com/yusing/goutils/http"
)

func TestIfElseBlockCommandServeHTTP_UnconditionalNilDoFallsThrough(t *testing.T) {
	elseCalled := false
	cmd := IfElseBlockCommand{
		Ifs: []IfBlockCommand{
			{
				On: RuleOn{},
				Do: nil,
			},
		},
		Else: []CommandHandler{
			Handler{
				fn: func(_ *httputils.ResponseModifier, _ *http.Request, _ http.HandlerFunc) error {
					elseCalled = true
					return nil
				},
				phase: PhaseNone,
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	rm := httputils.NewResponseModifier(w)

	err := cmd.ServeHTTP(rm, req, nil)
	require.NoError(t, err)
	assert.True(t, elseCalled)
}

func TestIfElseBlockCommandServeHTTP_ConditionalMatchedNilDoFallsThrough(t *testing.T) {
	elseCalled := false
	cmd := IfElseBlockCommand{
		Ifs: []IfBlockCommand{
			{
				On: RuleOn{
					checker: CheckFunc(func(_ *httputils.ResponseModifier, _ *http.Request) bool {
						return true
					}),
				},
				Do: nil,
			},
		},
		Else: []CommandHandler{
			Handler{
				fn: func(_ *httputils.ResponseModifier, _ *http.Request, _ http.HandlerFunc) error {
					elseCalled = true
					return nil
				},
				phase: PhaseNone,
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	rm := httputils.NewResponseModifier(w)

	err := cmd.ServeHTTP(rm, req, nil)
	require.NoError(t, err)
	assert.True(t, elseCalled)
}
