package rules

import (
	"strings"
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

func parseSimple(v string) (subject string, args []string, err error, ok bool) {
	brackets := 0
	for i := range len(v) {
		switch v[i] {
		case '\\', '$', '"', '\'', '`', '\t', '\r', '\n':
			return "", nil, nil, false
		case '(':
			brackets++
		case ')':
			if brackets == 0 {
				return "", nil, ErrUnterminatedBrackets, true
			}
			brackets--
		}
	}
	if brackets != 0 {
		return "", nil, ErrUnterminatedBrackets, true
	}

	i := 0
	for i < len(v) && v[i] == ' ' {
		i++
	}
	if i >= len(v) {
		return "", nil, nil, true
	}

	start := i
	for i < len(v) && v[i] != ' ' {
		i++
	}
	subject = v[start:i]

	if i >= len(v) {
		return subject, nil, nil, true
	}

	argCount := 0
	for j := i; j < len(v); {
		for j < len(v) && v[j] == ' ' {
			j++
		}
		if j >= len(v) {
			break
		}
		argCount++
		for j < len(v) && v[j] != ' ' {
			j++
		}
	}
	if argCount == 0 {
		return subject, nil, nil, true
	}
	args = make([]string, 0, argCount)
	for i < len(v) {
		for i < len(v) && v[i] == ' ' {
			i++
		}
		if i >= len(v) {
			break
		}
		start = i
		for i < len(v) && v[i] != ' ' {
			i++
		}
		args = append(args, v[start:i])
	}
	return subject, args, nil, true
}

// parse expression to subject and args
// with support for quotes, escaped chars, and env substitution, e.g.
//
//	error 403 "Forbidden 'foo' 'bar'"
//	error 403 Forbidden\ \"foo\"\ \"bar\".
//	error 403 "Message: ${CLOUDFLARE_API_KEY}"
func parse(v string) (subject string, args []string, err error) {
	if subject, args, err, ok := parseSimple(v); ok {
		return subject, args, err
	}

	buf := getStringBuffer(len(v))
	args = make([]string, 0, 4)

	escaped := false
	quote := rune(0)
	brackets := 0

	var (
		envVar         strings.Builder
		missingEnvVars []string
	)
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
				buf.WriteRune('\\')
				buf.WriteRune(r)
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

	switch {
	case quote != 0:
		err = ErrUnterminatedQuotes
	case brackets != 0:
		err = ErrUnterminatedBrackets
	case inEnvVar:
		err = ErrUnterminatedEnvVar
	default:
		flush(false)
	}
	if len(missingEnvVars) > 0 {
		err = gperr.Join(err, ErrEnvVarNotFound.With(gperr.Multiline().AddStrings(missingEnvVars...)))
	}
	return subject, args, err
}
