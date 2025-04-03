package ansi

import "regexp"

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

func StripANSI(s string) string {
	return ansiRegexp.ReplaceAllString(s, "")
}
