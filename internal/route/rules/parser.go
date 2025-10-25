package rules

import (
	"bytes"
	"fmt"
	"unicode"

	"github.com/yusing/goutils/env"
	gperr "github.com/yusing/goutils/errs"
)

var escapedChars = map[rune]rune{
	'n':  '\n',
	't':  '\t',
	'r':  '\r',
	'\'': '\'',
	'"':  '"',
	'\\': '\\',
	' ':  ' ',
}

var quoteChars = [256]bool{
	'"':  true,
	'\'': true,
	'`':  true,
}

// parse expression to subject and args
// with support for quotes, escaped chars, and env substitution, e.g.
//
//	error 403 "Forbidden 'foo' 'bar'"
//	error 403 Forbidden\ \"foo\"\ \"bar\".
//	error 403 "Message: ${CLOUDFLARE_API_KEY}"
func parse(v string) (subject string, args []string, err gperr.Error) {
	buf := bytes.NewBuffer(make([]byte, 0, len(v)))

	escaped := false
	quote := rune(0)
	brackets := 0

	var envVar bytes.Buffer
	var missingEnvVars []string
	inEnvVar := false
	expectingBrace := false

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
				fmt.Fprintf(buf, `\%c`, r)
			}
			escaped = false
			continue
		}
		if expectingBrace && r != '{' && r != '$' { // not escaped and not env var
			buf.WriteRune('$')
			expectingBrace = false
		}
		if quoteChars[r] {
			switch {
			case quote == 0 && brackets == 0:
				quote = r
				flush(false)
			case r == quote:
				quote = 0
				flush(true)
			default:
				buf.WriteRune(r)
			}
			continue
		}
		switch r {
		case '\\':
			escaped = true
		case '$':
			if expectingBrace { // $$ => $ and continue
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
				// NOTE: use env.LookupEnv instead of os.LookupEnv to support environment variable prefixes
				// like ${API_ADDR} will lookup for GODOXY_API_ADDR, GOPROXY_API_ADDR and API_ADDR.
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
		case '(':
			brackets++
			buf.WriteRune(r)
		case ')':
			if brackets == 0 {
				err = ErrUnterminatedBrackets
				return subject, args, err
			}
			brackets--
			buf.WriteRune(r)
		case ' ':
			if quote == 0 {
				flush(false)
			} else {
				buf.WriteRune(r)
			}
		default:
			if expectingBrace { // last was $ but { not matched
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

	if quote != 0 {
		err = ErrUnterminatedQuotes
	} else if brackets != 0 {
		err = ErrUnterminatedBrackets
	} else if inEnvVar {
		err = ErrUnterminatedEnvVar
	} else {
		flush(false)
	}
	if len(missingEnvVars) > 0 {
		err = gperr.Join(err, ErrEnvVarNotFound.With(gperr.Multiline().AddStrings(missingEnvVars...)))
	}
	return subject, args, err
}
