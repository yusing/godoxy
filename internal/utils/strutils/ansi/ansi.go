package ansi

import (
	"regexp"
)

var ansiRegexp = regexp.MustCompile(`\x1b\[[0-9;]*m`)

const (
	BrightRed    = "\x1b[91m"
	BrightGreen  = "\x1b[92m"
	BrightYellow = "\x1b[93m"
	BrightCyan   = "\x1b[96m"
	BrightWhite  = "\x1b[97m"
	Bold         = "\x1b[1m"
	Reset        = "\x1b[0m"

	HighlightRed    = BrightRed + Bold
	HighlightGreen  = BrightGreen + Bold
	HighlightYellow = BrightYellow + Bold
	HighlightCyan   = BrightCyan + Bold
	HighlightWhite  = BrightWhite + Bold
)

func Error(s string) string {
	return WithANSI(s, HighlightRed)
}

func Success(s string) string {
	return WithANSI(s, HighlightGreen)
}

func Warning(s string) string {
	return WithANSI(s, HighlightYellow)
}

func Info(s string) string {
	return WithANSI(s, HighlightCyan)
}

func WithANSI(s string, ansi string) string {
	return ansi + s + Reset
}

func StripANSI(s string) string {
	return ansiRegexp.ReplaceAllString(s, "")
}
