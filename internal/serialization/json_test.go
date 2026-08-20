package serialization

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	strutils "github.com/yusing/goutils/strings"
)

func TestJSONV2DurationRoundTrip(t *testing.T) {
	type payload struct {
		D time.Duration `json:"d"`
	}
	in := payload{D: time.Second}
	b, err := strutils.MarshalJSON(in)
	require.NoError(t, err)
	require.JSONEq(t, `{"d":1000000000}`, string(b))

	var out payload
	require.NoError(t, strutils.UnmarshalJSON(b, &out))
	require.Equal(t, time.Second, out.D)
}

func TestJSONV2StringAndValid(t *testing.T) {
	s, err := strutils.MarshalJSONString(map[string]int{"a": 1})
	require.NoError(t, err)
	require.JSONEq(t, `{"a":1}`, s)
	require.True(t, strutils.ValidJSONString(s))
	require.True(t, strutils.ValidJSON([]byte(s)))
	require.False(t, strutils.ValidJSONString("{"))

	var got map[string]int
	require.NoError(t, strutils.UnmarshalJSONString(s, &got))
	require.Equal(t, 1, got["a"])
}

func TestJSONV2EncoderDecoder(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, strutils.NewJSONEncoder(&buf).Encode(map[string]int{"n": 2}))
	require.True(t, strings.HasSuffix(buf.String(), "\n"), "streaming encode should end with a newline")
	require.JSONEq(t, `{"n":2}`, strings.TrimSpace(buf.String()))

	var got map[string]int
	require.NoError(t, strutils.NewJSONDecoder(&buf).Decode(&got))
	require.Equal(t, 2, got["n"])
}

func TestJSONV2MarshalIndent(t *testing.T) {
	b, err := strutils.MarshalJSONIndent(map[string]int{"a": 1}, "", "  ")
	require.NoError(t, err)
	require.Equal(t, "{\n  \"a\": 1\n}", string(b))
}
