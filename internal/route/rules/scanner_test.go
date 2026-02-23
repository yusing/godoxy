package rules

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTokenizer_skipComments_UnterminatedBlockComment(t *testing.T) {
	tok := newTokenizer("/* unterminated")
	pos, err := tok.skipComments(0, true, true)
	require.Error(t, err)
	require.Equal(t, 0, pos)
}

func TestTokenizer_skipComments_SkipsLineAndBlockComments(t *testing.T) {
	src := "  // line\n  /* block */\n  # hash\n  default"
	tok := newTokenizer(src)
	pos, err := tok.skipComments(0, true, true)
	require.NoError(t, err)
	require.Equal(t, strings.Index(src, "default"), pos)
}

func TestTokenizer_scanToBrace_IgnoresQuotedBraces(t *testing.T) {
	src := "cond \"{\" {" // the brace inside quotes must be ignored
	tok := newTokenizer(src)
	bracePos, err := tok.scanToBrace(0)
	require.NoError(t, err)
	require.Equal(t, strings.LastIndex(src, "{"), bracePos)
}

func TestTokenizer_findMatchingBrace_IgnoresQuotedClosingBrace(t *testing.T) {
	src := `{ "}" }`
	tok := newTokenizer(src)
	endPos, err := tok.findMatchingBrace(1) // body starts after the first '{'
	require.NoError(t, err)
	require.Equal(t, strings.LastIndex(src, "}"), endPos)
}
