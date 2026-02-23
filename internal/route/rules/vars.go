package rules

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"unsafe"

	httputils "github.com/yusing/goutils/http"
)

// TODO: remove middleware/vars.go and use this instead

type (
	reqVarGetter  func(*http.Request) string
	respVarGetter func(*httputils.ResponseModifier) string
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
	voidResponseModifier = httputils.NewResponseModifier(httptest.NewRecorder())
	dummyRequest         = http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Path: "/"},
		Header: http.Header{},
	}
)

type bytesBufferLike interface {
	io.Writer
	WriteByte(c byte) error
	WriteString(s string) (int, error)
}

type bytesBufferAdapter struct {
	io.Writer
}

func (b bytesBufferAdapter) WriteByte(c byte) error {
	buf := [1]byte{c}
	_, err := b.Write(buf[:])
	return err
}

func (b bytesBufferAdapter) WriteString(s string) (int, error) {
	return b.Write(unsafe.Slice(unsafe.StringData(s), len(s))) // avoid copy
}

func asBytesBufferLike(w io.Writer) bytesBufferLike {
	switch w := w.(type) {
	case *bytes.Buffer:
		return w
	case bytesBufferLike:
		return w
	default:
		return bytesBufferAdapter{w}
	}
}

// ValidateVars validates the variables in the given string.
// It returns the phase that the variables require and an error if any error occurs.
//
// Possible errors:
// - ErrUnexpectedVar: if any invalid variable is found
// - ErrUnterminatedEnvVar: missing closing }
// - ErrUnterminatedQuotes: missing closing " or ' or `
// - ErrUnterminatedParenthesis: missing closing )
func ValidateVars(s string) (phase PhaseFlag, err error) {
	return ExpandVars(voidResponseModifier, &dummyRequest, s, io.Discard)
}

// ExpandVars expands the variables in the given string and writes the result to the given writer.
// It returns the phase that the variables require and an error if any error occurs.
//
// Possible errors:
// - ErrUnexpectedVar: if any invalid variable is found
// - ErrUnterminatedEnvVar: missing closing }
// - ErrUnterminatedQuotes: missing closing " or ' or `
// - ErrUnterminatedParenthesis: missing closing )
func ExpandVars(w *httputils.ResponseModifier, req *http.Request, src string, dstW io.Writer) (phase PhaseFlag, err error) {
	dst := asBytesBufferLike(dstW)
	for i := 0; i < len(src); i++ {
		ch := src[i]
		if ch != '$' {
			if err = dst.WriteByte(ch); err != nil {
				return phase, err
			}
			continue
		}

		// Look ahead
		if i+1 >= len(src) {
			return phase, ErrUnterminatedEnvVar
		}
		j := i + 1

		switch src[j] {
		case '$': // $$ -> literal '$'
			if err := dst.WriteByte('$'); err != nil {
				return phase, err
			}
			i = j
			continue
		case '{': // ${...} pass through as-is
			if _, err := dst.WriteString("${"); err != nil {
				return phase, err
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
				phase |= getter.phase
				args, nextIdx, err := extractArgs(src, j, name)
				if err != nil {
					return phase, err
				}
				i = nextIdx
				actual, err = getter.get(args, w, req)
				if err != nil {
					return phase, err
				}
			} else if getter, ok := staticReqVarSubsMap[name]; ok { // always available
				actual = getter(req)
			} else if getter, ok := staticRespVarSubsMap[name]; ok { // post response
				actual = getter(w)
				phase |= PhasePost
			} else {
				return phase, ErrUnexpectedVar.Subject(name)
			}
			if _, err := dst.WriteString(actual); err != nil {
				return phase, err
			}
			if isStatic {
				i = k - 1
			}
			continue
		}

		// No valid construct after '$'
		return phase, ErrUnterminatedEnvVar.Withf("around $ at position %d", j)
	}

	return phase, nil
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
