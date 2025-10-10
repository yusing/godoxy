package rules

import (
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gobwas/glob"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	gperr "github.com/yusing/goutils/errs"
	httputils "github.com/yusing/goutils/http"
)

type (
	ValidateFunc      func(args []string) (any, gperr.Error)
	Tuple[T1, T2 any] struct {
		First  T1
		Second T2
	}
	StrTuple        = Tuple[string, string]
	IntTuple        = Tuple[int, int]
	MapValueMatcher = Tuple[string, Matcher]
)

func (t *Tuple[T1, T2]) Unpack() (T1, T2) {
	return t.First, t.Second
}

func (t *Tuple[T1, T2]) String() string {
	return fmt.Sprintf("%v:%v", t.First, t.Second)
}

type (
	Matcher     func(string) bool
	MatcherType string
)

const (
	MatcherTypeString MatcherType = "string"
	MatcherTypeGlob   MatcherType = "glob"
	MatcherTypeRegex  MatcherType = "regex"
)

func unquoteExpr(s string) (string, gperr.Error) {
	if s == "" {
		return "", nil
	}
	switch s[0] {
	case '"', '\'', '`':
		if s[0] != s[len(s)-1] {
			return "", ErrUnterminatedQuotes
		}
		return s[1 : len(s)-1], nil
	default:
		return s, nil
	}
}

func ExtractExpr(s string) (matcherType MatcherType, expr string, err gperr.Error) {
	idx := strings.IndexByte(s, '(')
	if idx == -1 {
		return MatcherTypeString, s, nil
	}
	idxEnd := strings.LastIndexByte(s, ')')
	if idxEnd == -1 {
		return "", "", ErrUnterminatedBrackets
	}

	expr, err = unquoteExpr(s[idx+1 : idxEnd])
	if err != nil {
		return "", "", err
	}
	matcherType = MatcherType(strings.ToLower(s[:idx]))

	switch matcherType {
	case MatcherTypeGlob, MatcherTypeRegex, MatcherTypeString:
		return
	default:
		return "", "", ErrInvalidArguments.Withf("invalid matcher type: %s", matcherType)
	}
}

func ParseMatcher(expr string) (Matcher, gperr.Error) {
	t, expr, err := ExtractExpr(expr)
	if err != nil {
		return nil, err
	}
	switch t {
	case MatcherTypeString:
		return StringMatcher(expr)
	case MatcherTypeGlob:
		return GlobMatcher(expr)
	case MatcherTypeRegex:
		return RegexMatcher(expr)
	}
	// won't reach here
	return nil, ErrInvalidArguments.Withf("invalid matcher type: %s", t)
}

func StringMatcher(s string) (Matcher, gperr.Error) {
	return func(s2 string) bool {
		return s == s2
	}, nil
}

func GlobMatcher(expr string) (Matcher, gperr.Error) {
	g, err := glob.Compile(expr)
	if err != nil {
		return nil, ErrInvalidArguments.With(err)
	}
	return g.Match, nil
}

func RegexMatcher(expr string) (Matcher, gperr.Error) {
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, ErrInvalidArguments.With(err)
	}
	return re.MatchString, nil
}

// validateSingleMatcher returns Matcher with the matcher validated.
func validateSingleMatcher(args []string) (any, gperr.Error) {
	if len(args) != 1 {
		return nil, ErrExpectOneArg
	}
	return ParseMatcher(args[0])
}

// toStrTuple returns *StrTuple.
func toStrTuple(args []string) (any, gperr.Error) {
	if len(args) != 2 {
		return nil, ErrExpectTwoArgs
	}
	return &StrTuple{args[0], args[1]}, nil
}

// toKVOptionalVMatcher returns *MapValueMatcher that value is optional.
func toKVOptionalVMatcher(args []string) (any, gperr.Error) {
	switch len(args) {
	case 1:
		return &MapValueMatcher{args[0], nil}, nil
	case 2:
		m, err := ParseMatcher(args[1])
		if err != nil {
			return nil, err
		}
		return &MapValueMatcher{args[0], m}, nil
	default:
		return nil, ErrExpectKVOptionalV
	}
}

// validateURL returns types.URL with the URL validated.
func validateURL(args []string) (any, gperr.Error) {
	if len(args) != 1 {
		return nil, ErrExpectOneArg
	}
	u, err := nettypes.ParseURL(args[0])
	if err != nil {
		return nil, ErrInvalidArguments.With(err)
	}
	return u, nil
}

// validateAbsoluteURL returns types.URL with the URL validated.
func validateAbsoluteURL(args []string) (any, gperr.Error) {
	if len(args) != 1 {
		return nil, ErrExpectOneArg
	}
	u, err := nettypes.ParseURL(args[0])
	if err != nil {
		return nil, ErrInvalidArguments.With(err)
	}
	if u.Scheme == "" {
		u.Scheme = "http"
	}
	if u.Host == "" {
		return nil, ErrInvalidArguments.Withf("missing host")
	}
	return u, nil
}

// validateCIDR returns types.CIDR with the CIDR validated.
func validateCIDR(args []string) (any, gperr.Error) {
	if len(args) != 1 {
		return nil, ErrExpectOneArg
	}
	if !strings.Contains(args[0], "/") {
		args[0] += "/32"
	}
	_, ipnet, err := net.ParseCIDR(args[0])
	if err != nil {
		return nil, ErrInvalidArguments.With(err)
	}
	return ipnet, nil
}

// validateURLPath returns string with the path validated.
func validateURLPath(args []string) (any, gperr.Error) {
	if len(args) != 1 {
		return nil, ErrExpectOneArg
	}
	p := args[0]
	trailingSlash := len(p) > 1 && p[len(p)-1] == '/'
	p, _, _ = strings.Cut(p, "#")
	p = path.Clean(p)
	if len(p) == 0 {
		return nil, ErrInvalidArguments.Withf("empty path")
	}
	if trailingSlash {
		p += "/"
	}
	return p, nil
}

func validateURLPathMatcher(args []string) (any, gperr.Error) {
	path, err := validateURLPath(args)
	if err != nil {
		return nil, err
	}
	return ParseMatcher(path.(string))
}

// validateFSPath returns string with the path validated.
func validateFSPath(args []string) (any, gperr.Error) {
	if len(args) != 1 {
		return nil, ErrExpectOneArg
	}
	p := filepath.Clean(args[0])
	if _, err := os.Stat(p); err != nil {
		return nil, ErrInvalidArguments.With(err)
	}
	return p, nil
}

// validateMethod returns string with the method validated.
func validateMethod(args []string) (any, gperr.Error) {
	if len(args) != 1 {
		return nil, ErrExpectOneArg
	}
	method := strings.ToUpper(args[0])
	if !httputils.IsMethodValid(method) {
		return nil, ErrInvalidArguments.Subject(method)
	}
	return method, nil
}

// validateUserBCryptPassword returns *HashedCrendential with the password validated.
func validateUserBCryptPassword(args []string) (any, gperr.Error) {
	if len(args) != 2 {
		return nil, ErrExpectTwoArgs
	}
	return BCryptCrendentials(args[0], []byte(args[1])), nil
}

// validateModField returns CommandHandler with the field validated.
func validateModField(mod FieldModifier, args []string) (CommandHandler, gperr.Error) {
	setField, ok := modFields[args[0]]
	if !ok {
		return nil, ErrInvalidSetTarget.Subject(args[0])
	}
	validArgs, err := setField.validate(args[1:])
	if err != nil {
		return nil, err.Withf(setField.help.String())
	}
	modder := setField.builder(validArgs)
	switch mod {
	case ModFieldAdd:
		return modder.add, nil
	case ModFieldRemove:
		return modder.remove, nil
	}
	return modder.set, nil
}
