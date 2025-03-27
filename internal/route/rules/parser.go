package rules

import (
	"bytes"
	"unicode"

	"github.com/yusing/go-proxy/internal/gperr"
)

var escapedChars = map[rune]rune{
	'n':  '\n',
	't':  '\t',
	'r':  '\r',
	'\'': '\'',
	'"':  '"',
	'\\': '\\',
	'$':  '$',
	' ':  ' ',
}

// parse expression to subject and args
// with support for quotes and escaped chars, e.g.
//
//	error 403 "Forbidden 'foo' 'bar'"
//	error 403 Forbidden\ \"foo\"\ \"bar\".
func parse(v string) (subject string, args []string, err gperr.Error) {
	buf := bytes.NewBuffer(make([]byte, 0, len(v)))

	escaped := false
	quote := rune(0)
	flush := func(quoted bool) {
		part := buf.String()
		if !quoted {
			beg := 0
			for i, r := range part {
				if unicode.IsSpace(r) {
					beg = i + 1
				} else {
					break
				}
			}
			if beg == len(part) { // all spaces
				return
			}
			part = part[beg:] // trim leading spaces
		}
		if subject == "" {
			subject = part
		} else {
			args = append(args, part)
		}
		buf.Reset()
	}
	for _, r := range v {
		if escaped {
			if ch, ok := escapedChars[r]; ok {
				buf.WriteRune(ch)
			} else {
				err = ErrUnsupportedEscapeChar.Subjectf("\\%c", r)
				return
			}
			escaped = false
			continue
		}
		switch r {
		case '\\':
			escaped = true
			continue
		case '"', '\'':
			switch {
			case quote == 0:
				quote = r
				flush(false)
			case r == quote:
				quote = 0
				flush(true)
			default:
				buf.WriteRune(r)
			}
		case ' ':
			if quote == 0 {
				flush(false)
				continue
			}
			fallthrough
		default:
			buf.WriteRune(r)
		}
	}

	if quote != 0 {
		err = ErrUnterminatedQuotes
	} else {
		flush(false)
	}
	return
}
