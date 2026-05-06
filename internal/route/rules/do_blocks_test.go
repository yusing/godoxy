package rules

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestParseDoWithBlocks_CommandOptionBlockFlattensToPositionalArgs(t *testing.T) {
	handlers, err := parseDoWithBlocks(`
set {
  target: header
  field: X-Test
  value: from-block
}
`)
	require.NoError(t, err)
	require.Len(t, handlers, 1)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	rm := httputils.NewResponseModifier(w)

	err = handlers[0].ServeHTTP(rm, req, nil)
	require.NoError(t, err)
	assert.Equal(t, "from-block", req.Header.Get("X-Test"))
}

func TestParseDoWithBlocks_CommandOptionBlockSupportsQuotedScalars(t *testing.T) {
	handlers, err := parseDoWithBlocks(`
rewrite {
  from: /old
  to: /new
}
notify {
  level: info
  provider: test-provider
  title: "title $req_method"
  body: ` + "`body $req_url $status_code`" + `
}
`)
	require.NoError(t, err)
	require.Len(t, handlers, 2)
}

func TestParseDoWithBlocks_CommandOptionBlockRejectsInvalidShape(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantErr string
	}{
		{
			name: "inline args",
			src: `
notify info {
  provider: test-provider
  title: title
  body: body
}
`,
			wantErr: "inline",
		},
		{
			name: "missing field",
			src: `
notify {
  level: info
  provider: test-provider
  title: title
}
`,
			wantErr: "body",
		},
		{
			name: "unknown field",
			src: `
notify {
  level: info
  provider: test-provider
  title: title
  body: body
  color: red
}
`,
			wantErr: "color",
		},
		{
			name: "inline block body",
			src: `
route foo { error 400 "bad: request" }
`,
			wantErr: "invalid arguments",
		},
		{
			name: "non mapping",
			src: `
notify {
  - level
  - info
}
`,
			wantErr: "option line",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseDoWithBlocks(tt.src)
			require.Error(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tt.wantErr))
		})
	}
}

func TestParseDoWithBlocks_CommandNameAlsoMatcherKeepsNestedBlock(t *testing.T) {
	handlers, err := parseDoWithBlocks(`
route example {
  set {
    target: header
    field: X-Route-Matched
    value: "yes"
  }
}
`)
	require.NoError(t, err)
	require.Len(t, handlers, 1)
	require.IsType(t, IfBlockCommand{}, handlers[0])
}

func TestParseDoWithBlocks_NestedBlockWithQuotedColonBody(t *testing.T) {
	handlers, err := parseDoWithBlocks(`
route example {
  error 400 "bad: request"
}
`)
	require.NoError(t, err)
	require.Len(t, handlers, 1)
	require.IsType(t, IfBlockCommand{}, handlers[0])
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
