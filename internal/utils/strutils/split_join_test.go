package strutils_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	. "github.com/yusing/godoxy/internal/utils/strutils"
)

var alphaNumeric = func() string {
	var s strings.Builder
	for i := range 'z' - 'a' + 1 {
		s.WriteRune('a' + i)
		s.WriteRune('A' + i)
		s.WriteRune(',')
	}
	for i := range '9' - '0' + 1 {
		s.WriteRune('0' + i)
		s.WriteRune(',')
	}
	return s.String()
}()

func TestSplit(t *testing.T) {
	tests := map[string]rune{
		"":  0,
		"1": '1',
		",": ',',
	}
	for sep, rsep := range tests {
		t.Run(sep, func(t *testing.T) {
			expected := strings.Split(alphaNumeric, sep)
			require.Equal(t, expected, SplitRune(alphaNumeric, rsep))
			require.Equal(t, alphaNumeric, JoinRune(expected, rsep))
		})
	}
}

func BenchmarkSplitRune(b *testing.B) {
	for range b.N {
		SplitRune(alphaNumeric, ',')
	}
}

func BenchmarkSplitRuneStdlib(b *testing.B) {
	for range b.N {
		strings.Split(alphaNumeric, ",")
	}
}

func BenchmarkJoinRune(b *testing.B) {
	for range b.N {
		JoinRune(SplitRune(alphaNumeric, ','), ',')
	}
}

func BenchmarkJoinRuneStdlib(b *testing.B) {
	for range b.N {
		strings.Join(SplitRune(alphaNumeric, ','), ",")
	}
}
