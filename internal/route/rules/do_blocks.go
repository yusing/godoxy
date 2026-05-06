package rules

import (
	"fmt"
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
	return lineEndsWithUnquotedToken(src, lineStart, lineEnd) == '{'
}

func lineContinuationOperator(src string, lineStart int, lineEnd int) byte {
	token := lineEndsWithUnquotedToken(src, lineStart, lineEnd)
	switch token {
	case '|', '&':
		return token
	default:
		return 0
	}
}

func lineEndsWithUnquotedToken(src string, lineStart int, lineEnd int) byte {
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
	if quote != 0 {
		return 0
	}
	return lastSignificant
}

// parseDoWithBlocks parses a do-body containing plain command lines and nested blocks.
// It returns the outer command handlers and the require phase.
//
// A nested block is recognized when a logical header ends with an unquoted '{'.
// Logical headers may span lines using trailing '|' or '&', for example:
//
//	remote 127.0.0.1 |
//	remote 192.168.0.0/16 {
//	  set header X-Remote-Type private
//	}
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

	appendBlockCommand := func(header string, body string) (bool, error) {
		directive, args, err := parse(strings.TrimSpace(header))
		if err != nil {
			return false, err
		}

		builder, ok := commands[directive]
		if !ok {
			return false, ErrUnknownDirective.Subject(directive)
		}
		if len(args) > 0 {
			if _, ok := checkers[directive]; ok {
				if strings.TrimSpace(body) != "" && !strings.Contains(body, "\n") {
					return true, ErrInvalidBlockSyntax.Withf("expected block body on a new line after '{'")
				}
				return false, nil
			}
			if !bodyLooksLikeOptionBlock(body) {
				return false, nil
			}
			return true, ErrInvalidArguments.Withf("option block does not accept inline args")
		}

		flatArgs, err := parseCommandBlockArgs(builder.help, body)
		if err != nil {
			return true, gperr.PrependSubject(err, directive).With(builder.help.Error())
		}

		phase, validArgs, err := builder.validate(flatArgs)
		if err != nil {
			return true, gperr.PrependSubject(err, directive).With(builder.help.Error())
		}

		h := builder.build(validArgs)
		handlers = append(handlers, Handler{fn: h, phase: phase, terminate: builder.terminate})
		return true, nil
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

			logicalEnd := linePos
			for logicalEnd < length && src[logicalEnd] != '\n' {
				logicalEnd++
			}

			for linePos < length && lineContinuationOperator(src, linePos, logicalEnd) != 0 {
				nextPos := logicalEnd
				if nextPos < length && src[nextPos] == '\n' {
					nextPos++
				}
				for nextPos < length {
					c := rune(src[nextPos])
					if c == '\n' {
						nextPos++
						continue
					}
					if c == '\r' || unicode.IsSpace(c) {
						nextPos++
						continue
					}
					break
				}
				if nextPos >= length {
					break
				}
				logicalEnd = nextPos
				for logicalEnd < length && src[logicalEnd] != '\n' {
					logicalEnd++
				}
			}

			if linePos < length && lineEndsWithUnquotedOpenBrace(src, linePos, logicalEnd) {
				header, bracePos, headerErr := parseHeaderToBrace(src, linePos)
				if headerErr != nil {
					return nil, headerErr
				}
				if directive, _, parseErr := parse(strings.TrimSpace(header)); parseErr == nil {
					if _, ok := commands[directive]; ok {
						p := bracePos
						bodyStart := p + 1
						bodyEnd, ferr := findMatchingBrace(src, &p, bodyStart)
						if ferr != nil {
							return nil, ferr
						}
						handled, err := appendBlockCommand(header, src[bodyStart:bodyEnd])
						if err != nil {
							return nil, err
						}
						if handled {
							pos = p
							lineStart = false
							continue
						}
					}
				}

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
			if lerr := appendLineCommand(src[pos:logicalEnd]); lerr != nil {
				return nil, lerr
			}
			pos = logicalEnd
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

func parseCommandBlockArgs(help Help, body string) ([]string, error) {
	values := make(map[string]string, help.args.Len())
	for line := range strings.SplitSeq(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name, rawValue, ok := strings.Cut(line, ":")
		if !ok {
			return nil, ErrInvalidArguments.Withf("expected option line in the form `name: value`")
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, ErrInvalidArguments.Withf("expected option name before `:`")
		}
		if !help.args.Contains(name) {
			return nil, ErrInvalidArguments.Withf("unknown option %q", name)
		}
		if _, exists := values[name]; exists {
			return nil, ErrInvalidArguments.Withf("duplicate option %q", name)
		}
		value, err := parseCommandBlockScalar(rawValue)
		if err != nil {
			return nil, ErrInvalidArguments.Withf("%s: %v", name, err)
		}
		values[name] = value
	}

	args := make([]string, 0, help.args.Len())
	for name := range help.args.IterKeys {
		value, ok := values[name]
		if !ok {
			return nil, ErrInvalidArguments.Withf("missing option %q", name)
		}
		args = append(args, value)
	}
	return args, nil
}

func bodyLooksLikeOptionBlock(body string) bool {
	for line := range strings.SplitSeq(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return strings.Contains(line, ":")
	}
	return true
}

func parseCommandBlockScalar(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", nil
	}

	_, args, err := parse("arg " + v)
	if err != nil {
		return "", err
	}
	if len(args) != 1 {
		return "", fmt.Errorf("expected a single scalar value")
	}
	return args[0], nil
}
