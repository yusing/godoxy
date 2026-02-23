package rules

import (
	"strings"
	"unicode"

	gperr "github.com/yusing/goutils/errs"
)

// Tokenizer provides utilities for parsing rule syntax with proper handling
// of quotes, comments, and env vars.
//
// This is intentionally reusable by both the top-level rule block parser and
// the nested do-block parser.
type Tokenizer struct {
	src    string
	length int
}

// newTokenizer creates a tokenizer for the given source.
func newTokenizer(src string) Tokenizer {
	return Tokenizer{src: src, length: len(src)}
}

// skipComments skips whitespace, line comments, and block comments.
// It returns the new position and an error if a block comment is unterminated.
func (t *Tokenizer) skipComments(pos int, atLineStart bool, prevIsSpace bool) (int, gperr.Error) {
	for pos < t.length {
		c := t.src[pos]

		// Skip whitespace
		if unicode.IsSpace(rune(c)) {
			pos++
			atLineStart = false
			prevIsSpace = true
			continue
		}

		// Check for line comment: // or #
		if c == '/' {
			if pos+1 < t.length && t.src[pos+1] == '/' {
				// Skip to end of line
				for pos < t.length && t.src[pos] != '\n' {
					pos++
				}
				atLineStart = true
				prevIsSpace = true
				continue
			}
		}
		if c == '#' && (atLineStart || prevIsSpace) {
			// Skip to end of line
			for pos < t.length && t.src[pos] != '\n' {
				pos++
			}
			atLineStart = true
			prevIsSpace = true
			continue
		}

		// Check for block comment: /*
		if c == '/' && pos+1 < t.length && t.src[pos+1] == '*' {
			pos += 2
			closed := false
			for pos+1 < t.length {
				if t.src[pos] == '*' && t.src[pos+1] == '/' {
					pos += 2
					closed = true
					break
				}
				pos++
			}
			if !closed {
				return 0, ErrInvalidBlockSyntax.Withf("unterminated block comment")
			}
			atLineStart = false
			prevIsSpace = true
			continue
		}

		break
	}

	return pos, nil
}

// scanToBrace scans from pos until it finds '{' outside quotes, or returns an error.
func (t *Tokenizer) scanToBrace(pos int) (int, gperr.Error) {
	quote := rune(0)
	for pos < t.length {
		c := rune(t.src[pos])
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			pos++
			continue
		}
		if c == '"' || c == '\'' || c == '`' {
			quote = c
			pos++
			continue
		}
		if c == '{' {
			return pos, nil
		}
		if c == '}' {
			return 0, ErrInvalidBlockSyntax.Withf("unmatched '}' in block header")
		}
		pos++
	}
	return 0, ErrInvalidBlockSyntax.Withf("expected '{' after block header")
}

// findMatchingBrace finds the matching '}' for a '{' starting at startPos.
// It respects quotes/backticks and ${...} env vars.
func (t *Tokenizer) findMatchingBrace(startPos int) (int, gperr.Error) {
	pos := startPos
	braceDepth := 1
	quote := rune(0)
	inLine := false
	inBlock := false
	atLineStart := true
	prevIsSpace := true

	for pos < t.length {
		c := rune(t.src[pos])

		if inLine {
			if c == '\n' {
				inLine = false
				atLineStart = true
				prevIsSpace = true
			}
			pos++
			continue
		}
		if inBlock {
			if c == '*' && pos+1 < t.length && t.src[pos+1] == '/' {
				pos += 2
				inBlock = false
				continue
			}
			if c == '\n' {
				atLineStart = true
				prevIsSpace = true
			}
			pos++
			continue
		}

		if quote != 0 {
			if c == quote {
				quote = 0
			}
			if c == '\n' {
				atLineStart = true
				prevIsSpace = true
			} else {
				atLineStart = false
				prevIsSpace = unicode.IsSpace(c)
			}
			pos++
			continue
		}

		if c == '"' || c == '\'' || c == '`' {
			quote = c
			atLineStart = false
			prevIsSpace = false
			pos++
			continue
		}

		// Comments (only outside quotes) at token boundary
		if c == '#' && (atLineStart || prevIsSpace) {
			inLine = true
			pos++
			continue
		}
		if c == '/' && pos+1 < t.length {
			n := rune(t.src[pos+1])
			if (atLineStart || prevIsSpace) && n == '/' {
				inLine = true
				pos += 2
				continue
			}
			if (atLineStart || prevIsSpace) && n == '*' {
				inBlock = true
				pos += 2
				continue
			}
		}

		if c == '$' && pos+1 < t.length && t.src[pos+1] == '{' {
			// Skip env var ${...}
			pos += 2
			envBraceDepth := 1
			envQuote := rune(0)
			for pos < t.length {
				ec := rune(t.src[pos])
				if envQuote != 0 {
					if ec == envQuote {
						envQuote = 0
					}
					pos++
					continue
				}
				if ec == '"' || ec == '\'' || ec == '`' {
					envQuote = ec
					pos++
					continue
				}
				if ec == '{' {
					envBraceDepth++
				} else if ec == '}' {
					envBraceDepth--
					if envBraceDepth == 0 {
						pos++ // Move past the closing '}'
						break
					}
				}
				pos++
			}
			continue
		}

		switch c {
		case '{':
			braceDepth++
		case '}':
			braceDepth--
			if braceDepth == 0 {
				return pos, nil
			}
		}

		if c == '\n' {
			atLineStart = true
			prevIsSpace = true
		} else {
			atLineStart = false
			prevIsSpace = unicode.IsSpace(c)
		}
		pos++
	}

	return 0, ErrInvalidBlockSyntax.Withf("unmatched '{' at position %d", startPos)
}

// parseHeaderToBrace parses an expression/header starting at start and returns:
//   - header: trimmed src[start:bracePos]
//   - bracePos: position of '{' (outside quotes/backticks)
func parseHeaderToBrace(src string, start int) (header string, bracePos int, err gperr.Error) {
	t := newTokenizer(src)
	bracePos, err = t.scanToBrace(start)
	if err != nil {
		return "", 0, err
	}
	return strings.TrimSpace(src[start:bracePos]), bracePos, nil
}

// findMatchingBrace finds the matching '}' for a '{' at position startPos.
// It respects quotes/backticks and ${...} env vars in do_body.
func findMatchingBrace(src string, pos *int, startPos int) (int, gperr.Error) {
	t := newTokenizer(src)
	endPos, err := t.findMatchingBrace(startPos)
	if err != nil {
		return 0, err
	}
	*pos = endPos + 1
	return endPos, nil
}
