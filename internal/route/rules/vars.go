package rules

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"

	ioutils "github.com/yusing/goutils/io"
)

// TODO: remove middleware/vars.go and use this instead

type (
	reqVarGetter  func(*http.Request) string
	respVarGetter func(*ResponseModifier) string
)

var reVar = regexp.MustCompile(`\$[\w_]+`)

var validVarNameCharset = func() (ret [256]bool) {
	for c := byte('a'); c <= 'z'; c++ {
		ret[c] = true
	}
	for c := byte('A'); c <= 'Z'; c++ {
		ret[c] = true
	}
	ret['_'] = true
	return
}()

func NeedExpandVars(s string) bool {
	return reVar.MatchString(s)
}

var (
	voidResponseModifier = NewResponseModifier(httptest.NewRecorder())
	dummyRequest         = http.Request{
		Method: "GET",
		URL:    &url.URL{Path: "/"},
		Header: http.Header{},
	}
)

// ValidateVars validates the variables in the given string.
// It returns ErrUnexpectedVar if any invalid variable is found.
func ValidateVars(s string) error {
	return ExpandVars(voidResponseModifier, &dummyRequest, s, io.Discard)
}

func ExpandVars(w *ResponseModifier, req *http.Request, src string, dstW io.Writer) error {
	dst := ioutils.NewBufferedWriter(dstW, 1024)
	defer dst.Close()

	for i := 0; i < len(src); i++ {
		ch := src[i]
		if ch != '$' {
			if err := dst.WriteByte(ch); err != nil {
				return err
			}
			continue
		}

		// Look ahead
		if i+1 >= len(src) {
			return ErrUnterminatedEnvVar
		}
		j := i + 1

		switch src[j] {
		case '$': // $$ -> literal '$'
			if err := dst.WriteByte('$'); err != nil {
				return err
			}
			i = j
			continue
		case '{': // ${...} pass through as-is
			if _, err := dst.WriteString("${"); err != nil {
				return err
			}
			i = j // we've consumed the '{' too
			continue
		}

		if validVarNameCharset[src[j]] {
			k := j
			for k < len(src) {
				c := src[k]
				if validVarNameCharset[c] {
					k++
					continue
				}
				break
			}
			name := src[j:k]
			isStatic := true

			var actual string
			if getter, ok := dynamicVarSubsMap[name]; ok {
				// Function-like variables
				isStatic = false
				args, nextIdx, err := extractArgs(src, j, name)
				if err != nil {
					return err
				}
				i = nextIdx
				actual, err = getter(args, w, req)
				if err != nil {
					return err
				}
			} else if getter, ok := staticReqVarSubsMap[name]; ok {
				actual = getter(req)
			} else if getter, ok := staticRespVarSubsMap[name]; ok {
				actual = getter(w)
			} else {
				return ErrUnexpectedVar.Subject(name)
			}
			if _, err := dst.WriteString(actual); err != nil {
				return err
			}
			if isStatic {
				i = k - 1
			}
			continue
		}

		// No valid construct after '$'
		return ErrUnterminatedEnvVar.Withf("around $ at position %d", j)
	}

	return nil
}

func extractArgs(src string, i int, funcName string) (args []string, nextIdx int, err error) {
	// Find opening parenthesis
	parenIdx := strings.IndexByte(src[i:], '(')
	if parenIdx == -1 {
		return nil, 0, ErrUnterminatedParenthesis.Withf("func %q at position %d", funcName, i)
	}
	parenIdx += i

	var (
		quote byte // current quote character (0 if not in quotes)
		arg   strings.Builder
	)

	nextIdx = parenIdx + 1
	for nextIdx < len(src) {
		ch := src[nextIdx]

		if quote != 0 {
			// We're inside a quoted string
			if ch == quote {
				// Closing quote - the content between quotes is now complete, add it
				args = append(args, arg.String())
				arg.Reset()
				quote = 0
				nextIdx++
				continue
			}
			// Inside quotes - add everything as-is
			arg.WriteByte(ch)
			nextIdx++
			continue
		}

		// Not inside quotes
		if quoteChars[ch] {
			// Opening quote
			quote = ch
			nextIdx++
			continue
		}

		if ch == ')' {
			// End of arguments
			if arg.Len() > 0 {
				args = append(args, arg.String())
			}
			return args, nextIdx, nil
		}

		if ch == ',' {
			// Argument separator
			if arg.Len() > 0 {
				args = append(args, arg.String())
				arg.Reset()
			}
			nextIdx++
			continue
		}

		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			// Whitespace outside quotes - skip
			nextIdx++
			continue
		}

		// Regular character - accumulate until we hit a delimiter
		arg.WriteByte(ch)
		nextIdx++
	}

	// Reached end of string without closing parenthesis
	if quote != 0 {
		return nil, 0, ErrUnterminatedQuotes.Withf("func %q", funcName)
	}
	return nil, 0, ErrUnterminatedParenthesis.Withf("func %q", funcName)
}
