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
	reqVarGetter func(*http.Request) string
	reqVar       struct {
		help Help
		get  reqVarGetter
	}
	respVarGetter func(*httputils.ResponseModifier) string
	respVar       struct {
		help Help
		get  respVarGetter
	}
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
				// Expand any nested $func(...) expressions in args
				args, argPhase, err := expandArgs(args, w, req)
				if err != nil {
					return phase, err
				}
				phase |= argPhase
				actual, err = getter.get(args, w, req)
				if err != nil {
					return phase, err
				}
			} else if getter, ok := staticReqVarSubsMap[name]; ok { // always available
				actual = getter.get(req)
			} else if getter, ok := staticRespVarSubsMap[name]; ok { // post response
				actual = getter.get(w)
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

		// Nested function call: $func(...) as an argument
		if ch == '$' && arg.Len() == 0 {
			// Capture the entire $func(...) expression as a raw argument token
			nestedEnd, nestedErr := extractNestedFuncExpr(src, nextIdx)
			if nestedErr != nil {
				return nil, 0, nestedErr
			}
			args = append(args, src[nextIdx:nestedEnd+1])
			nextIdx = nestedEnd + 1
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

// extractNestedFuncExpr finds the end index (inclusive) of a $func(...) expression
// starting at position start in src. It handles nested parentheses.
func extractNestedFuncExpr(src string, start int) (endIdx int, err error) {
	// src[start] must be '$'
	i := start + 1
	// skip the function name (valid var name chars)
	for i < len(src) && validVarNameCharset[src[i]] {
		i++
	}
	if i >= len(src) || src[i] != '(' {
		return 0, ErrUnterminatedParenthesis.Withf("nested func at position %d", start)
	}
	// Now find the matching closing parenthesis, respecting quotes and nesting
	depth := 0
	var quote byte
	for i < len(src) {
		ch := src[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			i++
			continue
		}
		if quoteChars[ch] {
			quote = ch
			i++
			continue
		}
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i, nil
			}
		}
		i++
	}
	if quote != 0 {
		return 0, ErrUnterminatedQuotes.Withf("nested func at position %d", start)
	}
	return 0, ErrUnterminatedParenthesis.Withf("nested func at position %d", start)
}

// expandArgs expands any args that are nested dynamic var expressions (starting with '$').
// It returns the expanded args and the combined phase flags.
func expandArgs(args []string, w *httputils.ResponseModifier, req *http.Request) (expanded []string, phase PhaseFlag, err error) {
	expanded = make([]string, len(args))
	for i, arg := range args {
		if len(arg) > 0 && arg[0] == '$' {
			var buf strings.Builder
			var argPhase PhaseFlag
			argPhase, err = ExpandVars(w, req, arg, &buf)
			if err != nil {
				return nil, phase, err
			}
			phase |= argPhase
			expanded[i] = buf.String()
		} else {
			expanded[i] = arg
		}
	}
	return expanded, phase, nil
}
