package rules

import (
	"net/http"
	"strings"
	"unicode"

	gperr "github.com/yusing/goutils/errs"
	httputils "github.com/yusing/goutils/http"
)

// IfBlockCommand is an inline conditional block inside a do-body.
//
// Syntax (within a rule do block):
//
//	<on-expr> { <do...> }
//
// Semantics:
//   - Evaluated in the same phase the parent rule runs.
//   - If <on-expr> matches, run the nested commands in-order.
//   - Otherwise do nothing.
//
// NOTE: Per current design decision, we keep this permissive:
// nested blocks may use response matchers and response commands; no extra phase validation is performed.
type IfBlockCommand struct {
	On RuleOn
	Do []CommandHandler
}

func (c IfBlockCommand) ServeHTTP(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
	if c.Do == nil {
		return nil
	}
	// If On.checker is nil, treat as unconditional (should not happen if parsed).
	if c.On.checker == nil {
		return Commands(c.Do).ServeHTTP(w, r, upstream)
	}
	if c.On.checker.Check(w, r) {
		return Commands(c.Do).ServeHTTP(w, r, upstream)
	}
	return nil
}

func (c IfBlockCommand) Phase() PhaseFlag {
	phase := c.On.phase
	for _, cmd := range c.Do {
		phase |= cmd.Phase()
	}
	return phase
}

// IfElseBlockCommand is a chained conditional block inside a do-body.
//
// Syntax (within a rule do block):
//
//	<on-expr> { <do...> } elif <on-expr> { <do...> } ... else { <do...> }
//
// NOTE: `elif`/`else` must appear on the same line as the preceding closing brace (`}`),
// e.g. `} elif ... {` and `} else {`.
type IfElseBlockCommand struct {
	Ifs  []IfBlockCommand
	Else []CommandHandler
}

func (c IfElseBlockCommand) ServeHTTP(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
	for _, br := range c.Ifs {
		// If On.checker is nil, treat as unconditional.
		if br.On.checker == nil {
			if br.Do == nil {
				return nil
			}
			return Commands(br.Do).ServeHTTP(w, r, upstream)
		}
		if br.On.checker.Check(w, r) {
			if br.Do == nil {
				return nil
			}
			return Commands(br.Do).ServeHTTP(w, r, upstream)
		}
	}
	if len(c.Else) > 0 {
		return Commands(c.Else).ServeHTTP(w, r, upstream)
	}
	return nil
}

func (c IfElseBlockCommand) Phase() PhaseFlag {
	phase := PhaseNone
	for _, br := range c.Ifs {
		phase |= br.Phase()
	}
	if len(c.Else) > 0 {
		phase |= Commands(c.Else).Phase()
	}
	return phase
}

func skipSameLineSpace(src string, pos int) int {
	for pos < len(src) {
		switch src[pos] {
		case '\n':
			return pos
		case '\r':
			pos++
			continue
		case ' ', '\t':
			pos++
			continue
		default:
			return pos
		}
	}
	return pos
}

func parseAtBlockChain(src string, blockPos int) (CommandHandler, int, error) {
	length := len(src)
	headerStart := blockPos

	parseBranch := func(onExpr string, bodyStart int, bodyEnd int) (RuleOn, []CommandHandler, error) {
		var on RuleOn
		if err := on.Parse(onExpr); err != nil {
			return RuleOn{}, nil, err
		}
		innerSrc := ""
		if bodyStart < bodyEnd {
			innerSrc = src[bodyStart:bodyEnd]
		}
		inner, err := parseDoWithBlocks(innerSrc)
		if err != nil {
			return RuleOn{}, nil, err
		}
		if len(inner) == 0 {
			return on, nil, nil
		}
		return on, inner, nil
	}

	onExpr, bracePos, herr := parseHeaderToBrace(src, headerStart)
	if herr != nil {
		return nil, 0, herr
	}
	if onExpr == "" {
		return nil, 0, ErrInvalidBlockSyntax.Withf("expected on-expr before '{'")
	}
	if bracePos >= length || src[bracePos] != '{' {
		return nil, 0, ErrInvalidBlockSyntax.Withf("expected '{' after nested block header")
	}

	// Parse first <on-expr> { ... }
	p := bracePos
	bodyStart := p + 1
	bodyEnd, ferr := findMatchingBrace(src, &p, bodyStart)
	if ferr != nil {
		return nil, 0, ferr
	}
	firstOn, firstDo, berr := parseBranch(onExpr, bodyStart, bodyEnd)
	if berr != nil {
		return nil, 0, berr
	}

	ifs := []IfBlockCommand{{On: firstOn, Do: firstDo}}
	var elseDo []CommandHandler
	hasChain := false
	hasElse := false

	for {
		q := skipSameLineSpace(src, p)
		if q >= length || src[q] == '\n' {
			break
		}

		// elif <on-expr> { ... }
		if strings.HasPrefix(src[q:], "elif") {
			next := q + len("elif")
			if next >= length {
				return nil, 0, ErrInvalidBlockSyntax.Withf("expected on-expr after 'elif'")
			}
			if src[next] == '\n' {
				return nil, 0, ErrInvalidBlockSyntax.Withf("expected on-expr after 'elif'")
			}
			if !unicode.IsSpace(rune(src[next])) {
				if src[next] == '{' || src[next] == '}' {
					return nil, 0, ErrInvalidBlockSyntax.Withf("expected on-expr after 'elif'")
				}
				return nil, 0, ErrInvalidBlockSyntax.Withf("expected whitespace after 'elif'")
			}
			next++
			for next < length {
				c := src[next]
				if c == '\n' {
					return nil, 0, ErrInvalidBlockSyntax.Withf("expected '{' after elif condition")
				}
				if c == '\r' {
					next++
					continue
				}
				if !unicode.IsSpace(rune(c)) {
					break
				}
				next++
			}

			p2 := next
			elifOnExpr, bracePos, herr := parseHeaderToBrace(src, p2)
			if herr != nil {
				return nil, 0, herr
			}
			if elifOnExpr == "" {
				return nil, 0, ErrInvalidBlockSyntax.Withf("expected on-expr after 'elif'")
			}
			if bracePos >= length || src[bracePos] != '{' {
				return nil, 0, ErrInvalidBlockSyntax.Withf("expected '{' after elif condition")
			}
			p2 = bracePos
			elifBodyStart := p2 + 1
			elifBodyEnd, ferr := findMatchingBrace(src, &p2, elifBodyStart)
			if ferr != nil {
				return nil, 0, ferr
			}
			elifOn, elifDo, berr := parseBranch(elifOnExpr, elifBodyStart, elifBodyEnd)
			if berr != nil {
				return nil, 0, berr
			}
			ifs = append(ifs, IfBlockCommand{On: elifOn, Do: elifDo})
			hasChain = true
			p = p2
			continue
		}

		// else { ... }
		if strings.HasPrefix(src[q:], "else") {
			if hasElse {
				return nil, 0, ErrInvalidBlockSyntax.Withf("multiple 'else' branches")
			}
			next := q + len("else")
			for next < length {
				c := src[next]
				if c == '\n' {
					return nil, 0, ErrInvalidBlockSyntax.Withf("expected '{' after 'else'")
				}
				if c == '\r' {
					next++
					continue
				}
				if !unicode.IsSpace(rune(c)) {
					break
				}
				next++
			}
			if next >= length || src[next] != '{' {
				return nil, 0, ErrInvalidBlockSyntax.Withf("expected '{' after 'else'")
			}

			elseBodyStart := next + 1
			p2 := next
			elseBodyEnd, ferr := findMatchingBrace(src, &p2, elseBodyStart)
			if ferr != nil {
				return nil, 0, ferr
			}
			innerSrc := ""
			if elseBodyStart < elseBodyEnd {
				innerSrc = src[elseBodyStart:elseBodyEnd]
			}
			inner, ierr := parseDoWithBlocks(innerSrc)
			if ierr != nil {
				return nil, 0, ierr
			}
			if len(inner) == 0 {
				elseDo = nil
			} else {
				elseDo = inner
			}
			hasChain = true
			hasElse = true
			p = p2

			// else must be the last branch on that line.
			for q2 := skipSameLineSpace(src, p); q2 < length && src[q2] != '\n'; q2 = skipSameLineSpace(src, q2) {
				return nil, 0, ErrInvalidBlockSyntax.Withf("unexpected token after else block")
			}
			break
		}

		return nil, 0, ErrInvalidBlockSyntax.Withf("unexpected token after nested block; expected 'elif'/'else' or newline")
	}

	if hasChain {
		return IfElseBlockCommand{Ifs: ifs, Else: elseDo}, p, nil
	}
	return IfBlockCommand{On: ifs[0].On, Do: ifs[0].Do}, p, nil
}

func lineEndsWithUnquotedOpenBrace(src string, lineStart int, lineEnd int) bool {
	quote := byte(0)
	lastSignificant := byte(0)
	atLineStart := true
	prevIsSpace := true

	for i := lineStart; i < lineEnd; i++ {
		c := src[i]
		if quote != 0 {
			if c == '\\' && i+1 < lineEnd {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			atLineStart = false
			prevIsSpace = false
			continue
		}
		if quoteChars[c] {
			quote = c
			atLineStart = false
			prevIsSpace = false
			continue
		}
		if c == '#' && (atLineStart || prevIsSpace) {
			break
		}
		if c == '/' && i+1 < lineEnd {
			n := rune(src[i+1])
			if (atLineStart || prevIsSpace) && (n == '/' || n == '*') {
				break
			}
		}
		if unicode.IsSpace(rune(c)) {
			prevIsSpace = true
			continue
		}
		lastSignificant = c
		atLineStart = false
		prevIsSpace = false
	}
	return quote == 0 && lastSignificant == '{'
}

// parseDoWithBlocks parses a do-body containing plain command lines and nested blocks.
// It returns the outer command handlers and the require phase.
//
// A nested block is recognized when a line ends with an unquoted '{' (ignoring trailing whitespace).
func parseDoWithBlocks(src string) (handlers []CommandHandler, err error) {
	pos := 0
	length := len(src)
	lineStart := true
	handlers = make([]CommandHandler, 0, strings.Count(src, "\n")+1)

	appendLineCommand := func(line string) error {
		line = strings.TrimSpace(line)
		if line == "" {
			return nil
		}

		directive, args, err := parse(line)
		if err != nil {
			return err
		}

		builder, ok := commands[directive]
		if !ok {
			return ErrUnknownDirective.Subject(directive)
		}

		phase, validArgs, err := builder.validate(args)
		if err != nil {
			return gperr.PrependSubject(err, directive).With(builder.help.Error())
		}

		h := builder.build(validArgs)
		handlers = append(handlers, Handler{fn: h, phase: phase, terminate: builder.terminate})
		return nil
	}

	for pos < length {
		// Handle newlines
		switch src[pos] {
		case '\n':
			pos++
			lineStart = true
			continue
		case '\r':
			// tolerate CRLF
			pos++
			continue
		}

		if lineStart {
			// Find first non-space on the line.
			linePos := pos
			for linePos < length {
				c := rune(src[linePos])
				if c == '\n' {
					break
				}
				if !unicode.IsSpace(c) {
					break
				}
				linePos++
			}

			lineEnd := linePos
			for lineEnd < length && src[lineEnd] != '\n' {
				lineEnd++
			}

			if linePos < length && lineEndsWithUnquotedOpenBrace(src, linePos, lineEnd) {
				h, next, err := parseAtBlockChain(src, linePos)
				if err != nil {
					return nil, err
				}
				handlers = append(handlers, h)
				pos = next
				lineStart = false
				continue
			}

			// Not a nested block; parse the rest of this line as a command.
			if lerr := appendLineCommand(src[pos:lineEnd]); lerr != nil {
				return nil, lerr
			}
			pos = lineEnd
			lineStart = true
			continue
		}

		// Not at line start; advance to the next line boundary.
		for pos < length && src[pos] != '\n' {
			pos++
		}
		lineStart = true
	}

	return handlers, nil
}
