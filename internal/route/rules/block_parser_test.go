package rules

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/serialization"
	httputils "github.com/yusing/goutils/http"
)

func testParseRules(t *testing.T, data string) Rules {
	t.Helper()

	var rules Rules
	convertible, err := serialization.ConvertString(data, reflect.ValueOf(&rules))
	require.True(t, convertible)
	require.NoError(t, err)
	return rules
}

func testParseRulesError(t *testing.T, data string) error {
	t.Helper()

	var rules Rules
	convertible, err := serialization.ConvertString(data, reflect.ValueOf(&rules))
	require.True(t, convertible)
	return err
}

func TestParseBlockRules_DefaultRule(t *testing.T) {
	rules := testParseRules(t, `default {
  upstream
}`)
	require.Len(t, rules, 1)
	assert.Equal(t, OnDefault, rules[0].On.raw)
	assert.Equal(t, "upstream", rules[0].Do.raw)
	assert.True(t, rules[0].Do.raw == CommandUpstream)
}

func TestParseBlockRules_ConditionalRule(t *testing.T) {
	rules := testParseRules(t, `path glob(/api/*) {
  proxy http://localhost:8080
}`)
	require.Len(t, rules, 1)
	assert.Equal(t, "path glob(/api/*)", rules[0].On.raw)
	assert.Equal(t, "proxy http://localhost:8080", rules[0].Do.raw)
	require.Len(t, rules[0].Do.pre, 1)
	_, ok := rules[0].Do.pre[0].(Handler)
	require.True(t, ok)
	require.Len(t, rules[0].Do.post, 0)
}

func TestParseBlockRules_MultipleRules(t *testing.T) {
	rules := testParseRules(t, `default {
  bypass
}

path /api/* {
  proxy http://localhost:8080
}

header Connection Upgrade &
header Upgrade websocket {
  route ws-api
  log info /dev/stdout "Websocket request $req_path from $remote_host to $upstream_name"
}`)
	require.Len(t, rules, 3)

	// Default rule
	assert.Equal(t, OnDefault, rules[0].On.raw)
	assert.Equal(t, "bypass", rules[0].Do.raw)

	// API rule
	assert.Equal(t, "path /api/*", rules[1].On.raw)
	assert.Equal(t, "proxy http://localhost:8080", rules[1].Do.raw)

	// WebSocket rule
	assert.Equal(t, "header Connection Upgrade &\nheader Upgrade websocket", rules[2].On.raw)
	assert.Equal(t, `route ws-api
  log info /dev/stdout "Websocket request $req_path from $remote_host to $upstream_name"`, rules[2].Do.raw)
	require.Len(t, rules[2].Do.pre, 2)
	_, ok := rules[2].Do.pre[0].(Handler)
	require.True(t, ok)
	_, ok = rules[2].Do.pre[1].(Handler)
	require.True(t, ok)
	require.Len(t, rules[2].Do.post, 0)
}

func TestParseBlockRules_Comments(t *testing.T) {
	rules := testParseRules(t, `// This is a comment
default {
  bypass // inline comment
}

/* Block comment
   spanning multiple lines */
path /admin/* {
  require_auth
}`)
	require.Len(t, rules, 2)
	assert.Equal(t, OnDefault, rules[0].On.raw)
	assert.Equal(t, "path /admin/*", rules[1].On.raw)
	assert.Equal(t, "require_auth", rules[1].Do.raw)
}

func TestParseBlockRules_HashComment(t *testing.T) {
	rules := testParseRules(t, `# YAML-style comment
default {
  bypass
}`)
	require.Len(t, rules, 1)
	assert.Equal(t, OnDefault, rules[0].On.raw)
	assert.Equal(t, "bypass", rules[0].Do.raw)
}

func TestParseBlockRules_EnvVars(t *testing.T) {
	t.Setenv("CUSTOM_HEADER", "test-header")

	rules := testParseRules(t, `path /api/* {
  set header X-Custom "${CUSTOM_HEADER}"
}`)
	require.Len(t, rules, 1)
	assert.Equal(t, "path /api/*", rules[0].On.raw)
	assert.Equal(t, `set header X-Custom "test-header"`, rules[0].Do.raw)
	require.Len(t, rules[0].Do.pre, 1)
	_, ok := rules[0].Do.pre[0].(Handler)
	require.True(t, ok)
	require.Len(t, rules[0].Do.post, 0)
}

func TestParseBlockRules_YAMLFallback(t *testing.T) {
	rules := testParseRules(t, `- name: default
  do: bypass
- name: api
  on: path glob(/api/*)
  do: proxy http://localhost:8080`)
	require.Len(t, rules, 2)
	assert.Equal(t, "default", rules[0].Name)
	assert.Equal(t, "bypass", rules[0].Do.raw)
	assert.Equal(t, "api", rules[1].Name)
	assert.Equal(t, "path glob(/api/*)", rules[1].On.raw)
	assert.Equal(t, "proxy http://localhost:8080", rules[1].Do.raw)
	require.Len(t, rules[1].Do.pre, 1)
	_, ok := rules[1].Do.pre[0].(Handler)
	require.True(t, ok)
	require.Len(t, rules[1].Do.post, 0)
}

func TestParseBlockRules_UnmatchedBrace(t *testing.T) {
	t.Run("unquoted", func(t *testing.T) {
		err := testParseRulesError(t, `path /api/* {
  proxy http://localhost:8080}
}`)
		require.Error(t, err)
	})
	t.Run("quoted", func(t *testing.T) {
		_ = testParseRules(t, `path /api/* {
			error 403 "some message}"
      }`)
	})
}

func TestParseBlockRules_UnterminatedBlockComment(t *testing.T) {
	err := testParseRulesError(t, `/* unterminated block comment
default {
  bypass
}`)
	require.Error(t, err)
}

func TestParseBlockRules_NestedBlocks(t *testing.T) {
	rules := testParseRules(t, `
header X-Test-Header {
  set header X-Remote-Type public
  remote 127.0.0.1 | remote 192.168.0.0/16 {
    set header X-Remote-Type private
  }
}`)

	require.Len(t, rules, 1)
	assert.Equal(t, "header X-Test-Header", rules[0].On.raw)

	require.Len(t, rules[0].Do.pre, 2)
	_, ok := rules[0].Do.pre[0].(Handler)
	require.True(t, ok)
	require.Len(t, rules[0].Do.post, 0)

	ifCmd, ok := rules[0].Do.pre[1].(IfBlockCommand)
	require.True(t, ok)
	assert.Equal(t, "remote 127.0.0.1 | remote 192.168.0.0/16", ifCmd.On.raw)

	require.Len(t, ifCmd.Do, 1)

	upstream := func(http.ResponseWriter, *http.Request) {}

	t.Run("condition matched executes nested content", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Test-Header", "1")
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		rm := httputils.NewResponseModifier(w)

		err := rules[0].Do.pre.ServeHTTP(rm, req, upstream)
		require.NoError(t, err)
		assert.Equal(t, "private", req.Header.Get("X-Remote-Type"))
	})

	t.Run("condition not matched skips nested content", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Test-Header", "1")
		req.RemoteAddr = "10.0.0.1:12345"
		w := httptest.NewRecorder()
		rm := httputils.NewResponseModifier(w)

		err := rules[0].Do.pre.ServeHTTP(rm, req, upstream)
		require.NoError(t, err)
		assert.Equal(t, "public", req.Header.Get("X-Remote-Type"))
	})
}

func TestParseBlockRules_NestedBlocks_ElifElse(t *testing.T) {
	rules := testParseRules(t, `
header X-Test-Header {
  set header X-Mode outer
  method GET {
    set header X-Mode get
  } elif method POST {
    set header X-Mode post
  } else {
    set header X-Mode other
  }
}`)

	require.Len(t, rules, 1)

	require.Len(t, rules[0].Do.pre, 2)

	ifCmd, ok := rules[0].Do.pre[1].(IfElseBlockCommand)
	require.True(t, ok)
	require.Len(t, ifCmd.Ifs, 2)
	assert.Equal(t, "method GET", ifCmd.Ifs[0].On.raw)
	assert.Equal(t, "method POST", ifCmd.Ifs[1].On.raw)
	require.NotNil(t, ifCmd.Else)

	upstream := func(http.ResponseWriter, *http.Request) {}
	cases := []struct {
		name   string
		method string
		want   string
	}{
		{name: "get branch", method: http.MethodGet, want: "get"},
		{name: "post branch", method: http.MethodPost, want: "post"},
		{name: "else branch", method: http.MethodPut, want: "other"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/", nil)
			req.Header.Set("X-Test-Header", "1")
			w := httptest.NewRecorder()
			rm := httputils.NewResponseModifier(w)

			err := rules[0].Do.pre.ServeHTTP(rm, req, upstream)
			require.NoError(t, err)
			assert.Equal(t, tc.want, req.Header.Get("X-Mode"))
		})
	}
}

func TestParseBlockRules_DefaultRule_CommentBeforeBrace(t *testing.T) {
	rules := testParseRules(t, `default /* comment between header and brace */ {
  bypass
}`)
	require.Len(t, rules, 1)
	assert.Equal(t, OnDefault, rules[0].On.raw)
	assert.Equal(t, "bypass", rules[0].Do.raw)
}

func TestParseBlockRules_StrayClosingBraceAtTopLevel(t *testing.T) {
	err := testParseRulesError(t, `}
default {
  bypass
}`)
	require.Error(t, err)
}

func TestParseBlockRules_NestedBlocks_ElifMustBeSameLine(t *testing.T) {
	err := testParseRulesError(t, `header X-Test-Header {
  method GET {
    set header X-Mode get
  }
  elif method POST {
    set header X-Mode post
  }
}`)
	require.Error(t, err)
}

func TestParseBlockRules_NestedBlocks_ElseMustBeLastOnLine(t *testing.T) {
	err := testParseRulesError(t, `header X-Test-Header {
  method GET {
    set header X-Mode get
  } else {
    set header X-Mode other
  } set header X-After else
}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected token after else block")
}

func TestParseBlockRules_NestedBlocks_MultipleElse(t *testing.T) {
	err := testParseRulesError(t, `header X-Test-Header {
  method GET {
    set header X-Mode get
  } else {
    set header X-Mode other
  } else {
    set header X-Mode other2
  }
}`)
	require.Error(t, err)
	// assert.Contains(t, err.Error(), "multiple 'else' branches")
	assert.Contains(t, err.Error(), "unexpected token after else block")
}

func TestParseBlockRules_NestedBlocks_ElifMissingOnExpr(t *testing.T) {
	err := testParseRulesError(t, `header X-Test-Header {
  method GET {
    set header X-Mode get
  } elif {
    set header X-Mode post
  }
}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected on-expr after 'elif'")
}

func TestParseBlockRules_NestedBlocks_LineEndingBraceHeuristic(t *testing.T) {
	rules := testParseRules(t, `{
  set header X-Literal "{"
}`)
	require.Len(t, rules, 1)
	require.Len(t, rules[0].Do.pre, 1)
	_, ok := rules[0].Do.pre[0].(Handler)
	require.True(t, ok)
}

func TestParseBlockRules_NestedBlocks_LineEndingBraceWithTrailingSpaces(t *testing.T) {
	rules := testParseRules(t, `header X-Test-Header {
  method GET {
    set header X-Mode get
  }
}`)
	require.Len(t, rules, 1)
	require.Len(t, rules[0].Do.pre, 1)
	ifCmd, ok := rules[0].Do.pre[0].(IfBlockCommand)
	require.True(t, ok)
	assert.Equal(t, "method GET", ifCmd.On.raw)
}

func TestParseBlockRules_NestedBlocks_LineEndingBraceWithTrailingComment(t *testing.T) {
	rules := testParseRules(t, `header X-Test-Header {
  method GET {    // GET branch
    set header X-Mode get
  } else {    # fallback branch
    set header X-Mode other
  }
}`)
	require.Len(t, rules, 1)
	require.Len(t, rules[0].Do.pre, 1)

	ifCmd, ok := rules[0].Do.pre[0].(IfElseBlockCommand)
	require.True(t, ok)
	require.Len(t, ifCmd.Ifs, 1)
	assert.Equal(t, "method GET", ifCmd.Ifs[0].On.raw)
	require.NotNil(t, ifCmd.Else)
}

func TestParseBlockRules_NestedBlocks_LineEndingBraceInterpretsAsBlock(t *testing.T) {
	err := testParseRulesError(t, `{
  set header X-Bad {
    set header X-Test fail
  }
}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid `rule.on` target")
}
