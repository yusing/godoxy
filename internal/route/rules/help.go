package rules

import (
	"fmt"
	"slices"
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
			start := strings.IndexByte(arg[pos:], '$')
			if start == -1 {
				if pos < len(arg) {
					// If no variable at all (pos == 0), cyan highlight for whole-arg
					// Otherwise, for mixed strings containing variables, leave non-variable text unhighlighted
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
				// Non-variable text should not be highlighted
				out.WriteString(arg[pos:start])
			}
			// Parse variable name and optional function call
			end := start + 1
			for end < len(arg) && (arg[end] == '_' || (arg[end] >= 'a' && arg[end] <= 'z') || (arg[end] >= 'A' && arg[end] <= 'Z') || (arg[end] >= '0' && arg[end] <= '9')) {
				end++
			}
			// Check for function call
			if end < len(arg) && arg[end] == '(' {
				parenCount := 1
				end++
				for end < len(arg) && parenCount > 0 {
					switch arg[end] {
					case '(':
						parenCount++
					case ')':
						parenCount--
					}
					end++
				}
			}
			varExpr := arg[start:end]
			out.WriteString(helpVar(varExpr))
			pos = end
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

// helpVar generates a highlighted string for a variable like "$req_method" or "$header(X-Test)"
func helpVar(varExpr string) string {
	if !strings.HasPrefix(varExpr, "$") {
		return varExpr
	}

	// Check if it's a function call
	parenIdx := strings.IndexByte(varExpr, '(')
	if parenIdx == -1 {
		// Simple variable like "$req_method"
		return ansi.WithANSI(varExpr, ansi.HighlightCyan)
	}

	// Function call like "$header(X-Test)"
	var sb strings.Builder
	sb.WriteString(ansi.WithANSI(varExpr[:parenIdx], ansi.HighlightCyan))
	sb.WriteString(ansi.WithANSI("(", ansi.HighlightWhite))

	// Extract and highlight the arguments
	argsStr := varExpr[parenIdx+1 : len(varExpr)-1]
	sb.WriteString(ansi.WithANSI(argsStr, ansi.HighlightYellow))

	sb.WriteString(ansi.WithANSI(")", ansi.HighlightWhite))
	return sb.String()
}

/*
Error generates help string as error, e.g.

	rewrite <from> <to>
		from: the path to rewrite, must start with /
		to: the path to rewrite to, must start with /
*/
func (h *Help) Error() gperr.Error {
	help := gperr.New(ansi.WithANSI(h.command, ansi.HighlightGreen))
	for _, line := range h.description {
		help = help.Withf("%s", line)
	}

	args := gperr.New("args")

	argKeys := make([]string, 0, len(h.args))
	longestArg := 0
	for arg := range h.args {
		if len(arg) > longestArg {
			longestArg = len(arg)
		}
		argKeys = append(argKeys, arg)
	}

	// sort argKeys alphabetically to make output stable
	slices.Sort(argKeys)
	for _, arg := range argKeys {
		desc := h.args[arg]
		paddedArg := fmt.Sprintf("%-"+strconv.Itoa(longestArg)+"s", arg)
		args = args.Withf("%s%s", ansi.WithANSI(paddedArg, ansi.HighlightCyan)+": ", desc)
	}

	return help.With(args)
}
