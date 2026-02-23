package rules

import (
	"strings"
	"unicode"

	"github.com/yusing/goutils/env"
	gperr "github.com/yusing/goutils/errs"
)

func getStringBuffer(size int) *strings.Builder {
	var buf strings.Builder
	if size > 0 {
		buf.Grow(size)
	}
	return &buf
}

// expandEnvVarsRaw expands ${NAME} in-place using env.LookupEnv (prefix-aware).
func expandEnvVarsRaw(v string) (string, gperr.Error) {
	buf := getStringBuffer(len(v))
	envVar := getStringBuffer(0)

	var missingEnvVars []string
	inEnvVar := false
	expectingBrace := false

	for _, r := range v {
		if expectingBrace && r != '{' && r != '$' {
			buf.WriteRune('$')
			expectingBrace = false
		}
		switch r {
		case '$':
			if expectingBrace {
				buf.WriteRune('$')
				expectingBrace = false
			} else {
				expectingBrace = true
			}
		case '{':
			if expectingBrace {
				inEnvVar = true
				expectingBrace = false
				envVar.Reset()
			} else {
				buf.WriteRune(r)
			}
		case '}':
			if inEnvVar {
				envValue, ok := env.LookupEnv(envVar.String())
				if !ok {
					missingEnvVars = append(missingEnvVars, envVar.String())
				} else {
					buf.WriteString(envValue)
				}
				inEnvVar = false
			} else {
				buf.WriteRune(r)
			}
		default:
			if expectingBrace {
				buf.WriteRune('$')
				expectingBrace = false
			}
			if inEnvVar {
				envVar.WriteRune(r)
			} else {
				buf.WriteRune(r)
			}
		}
	}

	if expectingBrace {
		buf.WriteRune('$')
	}

	var err gperr.Error
	if inEnvVar {
		// Write back the unterminated ${...} so the output matches the input.
		buf.WriteString("${")
		buf.WriteString(envVar.String())
		err = ErrUnterminatedEnvVar
	}
	if len(missingEnvVars) > 0 {
		err = gperr.Join(err, ErrEnvVarNotFound.With(gperr.Multiline().AddStrings(missingEnvVars...)))
	}
	return buf.String(), err
}

// parseBlockRules parses the block-syntax rule format.
// Grammar:
//
//	file        := { ws | comment | rule }
//	rule        := default_rule | conditional_rule
//	default_rule     := 'default' ws* block
//	conditional_rule := on_expr ws* block
//	block       := '{' do_body '}'
//
// Where:
//   - on_expr is passed verbatim to RuleOn.Parse()
//   - do_body is passed verbatim to Command.Parse()
//
// Comments (ignored outside quotes/backticks):
//   - line comment: // ... or # ...
//   - block comment: /* ... */
//
// Brace handling:
//   - Braces inside quotes/backticks are ignored
//   - Braces inside ${...} (env vars) are ignored in do_body
//   - Braces in on_expr are not ignored (env vars must be quoted in on_expr)
//
//nolint:dupword
func parseBlockRules(src string) (Rules, gperr.Error) {
	var rules Rules
	var errs gperr.Builder

	pos := 0
	length := len(src)
	t := newTokenizer(src)

	for pos < length {
		// Skip whitespace/comments between rules.
		newPos, skipErr := t.skipComments(pos, true, true)
		if skipErr != nil {
			return nil, ErrInvalidBlockSyntax.Withf("at position %d", pos)
		}
		pos = newPos
		if pos >= length {
			break
		}

		// Stray closing brace at top-level: keep parsing but mark invalid so Rules.Validate() fails.
		if src[pos] == '}' {
			return nil, ErrInvalidBlockSyntax.Withf("unmatched '}' at position %d", pos)
		}

		// Parse rule header (default, unconditional, or on_expr)
		headerStart := pos
		header := parseRuleHeader(&t, src, &pos, length)
		headerStr := src[headerStart:pos]

		// Skip whitespace/comments before '{' (default header may end before '{').
		newPos, skipErr = t.skipComments(pos, false, true)
		if skipErr != nil {
			return nil, ErrInvalidBlockSyntax.Withf("at position %d", pos)
		}
		pos = newPos

		if pos >= length || src[pos] != '{' {
			errs.AddSubjectf(ErrInvalidBlockSyntax, "expected '{' after rule header %q", headerStr)
			return nil, errs.Error()
		}

		// Find matching '}' (respecting quotes and env vars in do_body)
		bodyStart := pos + 1
		bodyEnd, err := t.findMatchingBrace(bodyStart)
		if err != nil {
			errs.AddSubjectf(err, "rule header %q", headerStr)
			return nil, errs.Error()
		}
		pos = bodyEnd + 1

		onExpr := header

		doBody := ""
		if bodyStart < bodyEnd {
			doBody = src[bodyStart:bodyEnd]
		}
		// Normalize do body for the inner DSL parser:
		// - strip comments (outside quotes/backticks)
		// - trim block whitespace/indentation
		// - expand ${ENV} in-place so cmd.raw is usable/debuggable
		doBody, err = preprocessDoBody(doBody)
		if err != nil {
			errs.AddSubjectf(err, "rule header %q", headerStr)
			return nil, errs.Error()
		}

		rule := Rule{
			Name: "", // auto-generate if empty
			On:   RuleOn{},
			Do:   Command{},
		}

		// Header semantics:
		// - "default" => default rule (matched when no other rules are matched)
		// - ""        => unconditional rule (always matches)
		// - otherwise  => conditional rule (on expression)
		switch onExpr {
		case "default":
			rule.On.raw = OnDefault
		case "":
			// leave rule.On as zero value => checker=nil => always matches
		default:
			if parseErr := rule.On.Parse(onExpr); parseErr != nil {
				errs.AddSubjectf(parseErr, "on")
			}
		}

		if doBody != "" {
			if parseErr := rule.Do.Parse(doBody); parseErr != nil {
				errs.AddSubjectf(parseErr, "do")
			}
		}

		if errs.HasError() {
			return nil, errs.Error()
		}

		rules = append(rules, rule)
	}

	return rules, nil
}

func preprocessDoBody(doBody string) (string, gperr.Error) {
	doBody = strings.TrimSpace(doBody)
	if doBody == "" {
		return "", nil
	}

	normalized := doBody
	// If comments are possible, strip them first while preserving line breaks.
	if strings.ContainsAny(normalized, "#/") {
		stripped, err := stripCommentsPreserveNewlines(normalized)
		if err != nil {
			return "", err
		}
		normalized = stripped
	}

	// Drop lines that are empty after trimming, while preserving indentation of non-empty lines.
	out := getStringBuffer(len(normalized))

	lineStart := 0
	wroteLine := false
	for i := 0; i <= len(normalized); i++ {
		if i < len(normalized) && normalized[i] != '\n' {
			continue
		}
		line := normalized[lineStart:i]
		if strings.TrimSpace(line) != "" {
			if wroteLine {
				out.WriteByte('\n')
			}
			out.WriteString(line)
			wroteLine = true
		}
		lineStart = i + 1
	}

	if !wroteLine {
		return "", nil
	}
	normalized = out.String()

	// Expand env vars to keep Command.raw consistent with parsed semantics.
	if !strings.Contains(normalized, "${") {
		return normalized, nil
	}
	expanded, err := expandEnvVarsRaw(normalized)
	if err != nil {
		return "", err
	}
	return expanded, nil
}

// stripCommentsPreserveNewlines removes //, #, and /* */ comments outside quotes/backticks.
// It preserves newlines so command line boundaries remain intact.
func stripCommentsPreserveNewlines(src string) (string, gperr.Error) {
	if !strings.ContainsAny(src, "#/") {
		return src, nil
	}

	out := getStringBuffer(len(src))

	quote := rune(0)
	inLine := false
	inBlock := false
	atLineStart := true
	prevIsSpace := true

	for i := 0; i < len(src); {
		c := src[i]

		if inLine {
			if c == '\n' {
				inLine = false
				out.WriteByte('\n')
				atLineStart = true
				prevIsSpace = true
			}
			i++
			continue
		}
		if inBlock {
			if c == '\n' {
				out.WriteByte('\n')
				atLineStart = true
				prevIsSpace = true
				i++
				continue
			}
			if c == '*' && i+1 < len(src) && src[i+1] == '/' {
				inBlock = false
				i += 2
				continue
			}
			i++
			continue
		}

		if quote != 0 {
			out.WriteByte(c)
			if c == '\\' && i+1 < len(src) {
				// Write next char and skip it (escape sequence)
				i++
				out.WriteByte(src[i])
				atLineStart = false
				prevIsSpace = false
				i++
				continue
			}
			if rune(c) == quote {
				quote = 0
			}
			if c == '\n' {
				atLineStart = true
				prevIsSpace = true
			} else {
				atLineStart = false
				prevIsSpace = unicode.IsSpace(rune(c))
			}
			i++
			continue
		}

		// Not in quote/comment.
		switch c {
		case '\'', '"', '`':
			quote = rune(c)
			out.WriteByte(c)
			atLineStart = false
			prevIsSpace = false
			i++
			continue
		case '#':
			if atLineStart || prevIsSpace {
				inLine = true
				i++
				continue
			}
		case '/':
			if i+1 < len(src) {
				n := src[i+1]
				if (atLineStart || prevIsSpace) && n == '/' {
					inLine = true
					i += 2
					continue
				}
				if (atLineStart || prevIsSpace) && n == '*' {
					inBlock = true
					i += 2
					continue
				}
			}
		}

		out.WriteByte(c)
		if c == '\n' {
			atLineStart = true
			prevIsSpace = true
		} else {
			atLineStart = false
			prevIsSpace = unicode.IsSpace(rune(c))
		}
		i++
	}

	if inBlock {
		return "", ErrInvalidBlockSyntax.Withf("unterminated block comment")
	}
	return out.String(), nil
}

// parseRuleHeader parses the rule header (default or on expression).
// Returns the header string, or "" if parsing failed.
func parseRuleHeader(t *Tokenizer, src string, pos *int, length int) string {
	start := *pos

	// Check for 'default' keyword
	if *pos+7 <= length && src[*pos:*pos+7] == "default" {
		next := *pos + 7
		if next >= length || unicode.IsSpace(rune(src[next])) {
			*pos = next
			return "default"
		}
	}

	// Parse on expression until we hit '{' outside quotes.
	bracePos, err := t.scanToBrace(*pos)
	if err != nil {
		*pos = length
		return strings.TrimSpace(src[start:*pos])
	}
	*pos = bracePos
	return strings.TrimSpace(src[start:*pos])
}
