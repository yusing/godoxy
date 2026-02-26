package rules

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	httputils "github.com/yusing/goutils/http"
)

func TestIfElseBlockCommandServeHTTP_UnconditionalNilDoNotFallsThrough(t *testing.T) {
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
	assert.False(t, elseCalled)
}

func TestIfElseBlockCommandServeHTTP_ConditionalMatchedNilDoNotFallsThrough(t *testing.T) {
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
	assert.False(t, elseCalled)
}

func TestParseDoWithBlocks_MultilineBlockHeaderContinuation(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "or continuation",
			src: `
remote 127.0.0.1 |
remote 192.168.0.0/16 {
  set header X-Remote-Type private
}
`,
		},
		{
			name: "and continuation",
			src: `
method GET &
remote 127.0.0.1 {
  set header X-Remote-Type private
}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handlers, err := parseDoWithBlocks(tt.src)
			require.NoError(t, err)
			require.Len(t, handlers, 1)
			require.IsType(t, IfBlockCommand{}, handlers[0])
		})
	}
}
