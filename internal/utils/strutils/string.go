package strutils

import (
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// CommaSeperatedList returns a list of strings split by commas,
// then trim spaces from each element.
func CommaSeperatedList(s string) []string {
	if s == "" {
		return []string{}
	}
	res := SplitComma(s)
	for i, part := range res {
		res[i] = strings.TrimSpace(part)
	}
	return res
}

var caseTitle = cases.Title(language.AmericanEnglish)

func Title(s string) string {
	return caseTitle.String(s)
}

func ContainsFold(s, substr string) bool {
	return IndexFold(s, substr) >= 0
}

func IndexFold(s, substr string) int {
	return strings.Index(strings.ToLower(s), strings.ToLower(substr))
}

func ToLowerNoSnake(s string) string {
	var buf strings.Builder
	for _, r := range s {
		if r == '_' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		buf.WriteRune(r)
	}
	return buf.String()
}

//nolint:intrange
func LevenshteinDistance(a, b string) int {
	if a == b {
		return 0
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	v0 := make([]int, len(b)+1)
	v1 := make([]int, len(b)+1)

	for i := 0; i <= len(b); i++ {
		v0[i] = i
	}

	for i := 0; i < len(a); i++ {
		v1[0] = i + 1

		for j := 0; j < len(b); j++ {
			cost := 0
			if a[i] != b[j] {
				cost = 1
			}

			v1[j+1] = min3(v1[j]+1, v0[j+1]+1, v0[j]+cost)
		}

		for j := 0; j <= len(b); j++ {
			v0[j] = v1[j]
		}
	}

	return v1[len(b)]
}

func min3(a, b, c int) int {
	if a < b && a < c {
		return a
	}
	if b < a && b < c {
		return b
	}
	return c
}
