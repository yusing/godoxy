package rules

import (
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
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
	Tuple3[T1, T2, T3 any] struct {
		First  T1
		Second T2
		Third  T3
	}
	Tuple4[T1, T2, T3, T4 any] struct {
		First  T1
		Second T2
		Third  T3
		Fourth T4
	}
	StrTuple        = Tuple[string, string]
	IntTuple        = Tuple[int, int]
	MapValueMatcher = Tuple[string, Matcher]
)

func (t *Tuple[T1, T2]) Unpack() (T1, T2) {
	return t.First, t.Second
}

func (t *Tuple3[T1, T2, T3]) Unpack() (T1, T2, T3) {
	return t.First, t.Second, t.Third
}

func (t *Tuple4[T1, T2, T3, T4]) Unpack() (T1, T2, T3, T4) {
	return t.First, t.Second, t.Third, t.Fourth
}

func (t *Tuple[T1, T2]) String() string {
	return fmt.Sprintf("%v:%v", t.First, t.Second)
}

func (t *Tuple3[T1, T2, T3]) String() string {
	return fmt.Sprintf("%v:%v:%v", t.First, t.Second, t.Third)
}

func (t *Tuple4[T1, T2, T3, T4]) String() string {
	return fmt.Sprintf("%v:%v:%v:%v", t.First, t.Second, t.Third, t.Fourth)
}

// validateSingleMatcher returns Matcher with the matcher validated.
func validateSingleMatcher(args []string) (any, gperr.Error) {
	if len(args) != 1 {
		return nil, ErrExpectOneArg
	}
	return ParseMatcher(args[0])
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

func toKeyValueTemplate(args []string) (any, gperr.Error) {
	if len(args) != 2 {
		return nil, ErrExpectTwoArgs
	}

	isTemplate, err := validateTemplate(args[1], false)
	if err != nil {
		return nil, err
	}
	return &keyValueTemplate{args[0], isTemplate}, nil
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
	if u.Scheme == "" {
		// expect relative URL, must starts with /
		if !strings.HasPrefix(u.Path, "/") {
			return nil, ErrInvalidArguments.Withf("relative URL must starts with /")
		}
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

func validateStatusCode(status string) (int, error) {
	statusCode, err := strconv.Atoi(status)
	if err != nil {
		return 0, err
	}
	if statusCode < 100 || statusCode > 599 {
		return 0, fmt.Errorf("status code out of range: %s", status)
	}
	return statusCode, nil
}

// validateStatusRange returns Tuple[int, int] with the status range validated.
// accepted formats are:
//   - <status>
//   - <status>-<status>
//   - 1xx
//   - 2xx
//   - 3xx
//   - 4xx
//   - 5xx
func validateStatusRange(args []string) (any, gperr.Error) {
	if len(args) != 1 {
		return nil, ErrExpectOneArg
	}

	beg, end, ok := strings.Cut(args[0], "-")
	if !ok { // <status>
		end = beg
	}

	switch beg {
	case "1xx":
		return &IntTuple{100, 199}, nil
	case "2xx":
		return &IntTuple{200, 299}, nil
	case "3xx":
		return &IntTuple{300, 399}, nil
	case "4xx":
		return &IntTuple{400, 499}, nil
	case "5xx":
		return &IntTuple{500, 599}, nil
	}

	begInt, begErr := validateStatusCode(beg)
	endInt, endErr := validateStatusCode(end)
	if begErr != nil || endErr != nil {
		return nil, ErrInvalidArguments.With(gperr.Join(begErr, endErr))
	}
	return &IntTuple{begInt, endInt}, nil
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
	if len(args) == 0 {
		return nil, ErrExpectTwoOrThreeArgs
	}
	setField, ok := modFields[args[0]]
	if !ok {
		return nil, ErrUnknownModField.Subject(args[0])
	}
	if mod == ModFieldRemove {
		if len(args) != 2 {
			return nil, ErrExpectTwoArgs
		}
		// setField expect validateStrTuple
		args = append(args, "")
	}
	validArgs, err := setField.validate(args[1:])
	if err != nil {
		return nil, err.With(setField.help.Error())
	}
	modder := setField.builder(validArgs)
	switch mod {
	case ModFieldAdd:
		add := modder.add
		if add == nil {
			return nil, ErrInvalidArguments.Withf("add is not supported for %s", mod)
		}
		return add, nil
	case ModFieldRemove:
		remove := modder.remove
		if remove == nil {
			return nil, ErrInvalidArguments.Withf("remove is not supported for %s", mod)
		}
		return remove, nil
	}
	set := modder.set
	if set == nil {
		return nil, ErrInvalidArguments.Withf("set is not supported for %s", mod)
	}
	return set, nil
}

func validateTemplate(tmplStr string, newline bool) (templateString, gperr.Error) {
	if newline && !strings.HasSuffix(tmplStr, "\n") {
		tmplStr += "\n"
	}

	if !NeedExpandVars(tmplStr) {
		return templateString{tmplStr, false}, nil
	}

	err := ValidateVars(tmplStr)
	if err != nil {
		return templateString{}, gperr.Wrap(err)
	}
	return templateString{tmplStr, true}, nil
}

func validateLevel(level string) (zerolog.Level, gperr.Error) {
	l, err := zerolog.ParseLevel(level)
	if err != nil {
		return zerolog.NoLevel, ErrInvalidArguments.With(err)
	}
	return l, nil
}

// func validateNotifProvider(provider string) gperr.Error {
// 	if !notif.HasProvider(provider) {
// 		return ErrInvalidArguments.Subject(provider)
// 	}
// 	return nil
// }
