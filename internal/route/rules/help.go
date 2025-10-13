package rules

import (
	"fmt"
	"strconv"
	"strings"

	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/strings/ansi"
)

type Help struct {
	command     string
	description []string
	args        map[string]string // args[arg] -> description
}

func makeLines(lines ...string) []string {
	return lines
}

func helpExample(cmd string, args ...string) string {
	var sb strings.Builder
	sb.WriteString("  ")
	sb.WriteString(ansi.WithANSI(cmd, ansi.HighlightGreen))
	for _, arg := range args {
		var out strings.Builder
		pos := 0
		for {
			start := strings.Index(arg[pos:], "{{")
			if start == -1 {
				if pos < len(arg) {
					// If no template at all (pos == 0), cyan highlight for whole-arg
					// Otherwise, for mixed strings containing templates, leave non-template text unhighlighted
					if pos == 0 {
						out.WriteString(ansi.WithANSI(arg[pos:], ansi.HighlightCyan))
					} else {
						out.WriteString(arg[pos:])
					}
				}
				break
			}
			start += pos
			if start > pos {
				// Non-template text should not be highlighted
				out.WriteString(arg[pos:start])
			}
			end := strings.Index(arg[start+2:], "}}")
			if end == -1 {
				// Unmatched template start; write remainder without highlighting
				out.WriteString(arg[start:])
				break
			}
			end += start + 2
			inner := strings.TrimSpace(arg[start+2 : end])
			parts := strings.Split(inner, ".")
			out.WriteString(helpTemplateVar(parts...))
			pos = end + 2
		}
		fmt.Fprintf(&sb, ` "%s"`, out.String())
	}
	return sb.String()
}

func helpListItem(key string, value string) string {
	var sb strings.Builder
	sb.WriteString("  ")
	sb.WriteString(ansi.WithANSI(key, ansi.HighlightYellow))
	sb.WriteString(": ")
	sb.WriteString(value)
	return sb.String()
}

// helpFuncCall generates a string like "fn(arg1, arg2, arg3)"
func helpFuncCall(fn string, args ...string) string {
	var sb strings.Builder
	sb.WriteString(ansi.WithANSI(fn, ansi.HighlightRed))
	sb.WriteString("(")
	for i, arg := range args {
		fmt.Fprintf(&sb, `"%s"`, ansi.WithANSI(arg, ansi.HighlightCyan))
		if i < len(args)-1 {
			sb.WriteString(", ")
		}
	}
	sb.WriteString(")")
	return sb.String()
}

// helpTemplateVar generates a string like "{{ .Request.Method }} {{ .Request.URL.Path }}"
func helpTemplateVar(parts ...string) string {
	var sb strings.Builder
	sb.WriteString(ansi.WithANSI("{{ ", ansi.HighlightWhite))
	for i, part := range parts {
		sb.WriteString(ansi.WithANSI(part, ansi.HighlightCyan))
		if i < len(parts)-1 {
			sb.WriteString(".")
		}
	}
	sb.WriteString(ansi.WithANSI(" }}", ansi.HighlightWhite))
	return sb.String()
}

/*
Generate help string as error, e.g.

	rewrite <from> <to>
		from: the path to rewrite, must start with /
		to: the path to rewrite to, must start with /
*/
func (h *Help) Error() gperr.Error {
	var lines gperr.MultilineError

	lines.Adds(ansi.WithANSI(h.command, ansi.HighlightGreen))
	lines.AddStrings(h.description...)
	lines.Adds("  args:")

	longestArg := 0
	for arg := range h.args {
		if len(arg) > longestArg {
			longestArg = len(arg)
		}
	}

	for arg, desc := range h.args {
		lines.Addf("    %-"+strconv.Itoa(longestArg)+"s: %s", ansi.WithANSI(arg, ansi.HighlightCyan), desc)
	}
	return &lines
}
