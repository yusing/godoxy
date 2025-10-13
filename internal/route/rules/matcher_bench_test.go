package rules

import "testing"

func BenchmarkMatcher(b *testing.B) {
	b.Run("StringMatcher", func(b *testing.B) {
		matcher, err := StringMatcher("foo")
		if err != nil {
			b.Fatal(err)
		}
		for b.Loop() {
			matcher("foo")
		}
	})

	b.Run("GlobMatcher", func(b *testing.B) {
		matcher, err := GlobMatcher("foo*bar?baz*[abc]*.txt")
		if err != nil {
			b.Fatal(err)
		}
		for b.Loop() {
			matcher("foooooobarzbazcb.txt")
		}
	})

	b.Run("RegexMatcher", func(b *testing.B) {
		matcher, err := RegexMatcher(`^(foo\d+|bar(_baz)?)[a-z]{3,}\.txt$`)
		if err != nil {
			b.Fatal(err)
		}
		for b.Loop() {
			matcher("foo123abcd.txt")
		}
	})
}
