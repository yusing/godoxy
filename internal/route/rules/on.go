package rules

import (
	"net"
	"net/http"
	"slices"
	"strings"

	"github.com/yusing/godoxy/internal/route/routes"
	gperr "github.com/yusing/goutils/errs"
	strutils "github.com/yusing/goutils/strings"
)

type RuleOn struct {
	raw               string
	checker           Checker
	isResponseChecker bool
}

func (on *RuleOn) IsResponseChecker() bool {
	return on.isResponseChecker
}

func (on *RuleOn) Check(w http.ResponseWriter, r *http.Request) bool {
	return on.checker.Check(w, r)
}

const (
	OnHeader    = "header"
	OnQuery     = "query"
	OnCookie    = "cookie"
	OnForm      = "form"
	OnPostForm  = "postform"
	OnMethod    = "method"
	OnHost      = "host"
	OnPath      = "path"
	OnRemote    = "remote"
	OnBasicAuth = "basic_auth"
	OnRoute     = "route"

	// on response
	OnResponseHeader = "resp_header"
	OnStatus         = "status"
)

var checkers = map[string]struct {
	help              Help
	validate          ValidateFunc
	builder           func(args any) CheckFunc
	isResponseChecker bool
}{
	OnHeader: {
		help: Help{
			command: OnHeader,
			description: makeLines(
				"Value supports string, glob pattern, or regex pattern, e.g.:",
				helpExample(OnHeader, "username", "user"),
				helpExample(OnHeader, "username", helpFuncCall("glob", "user*")),
				helpExample(OnHeader, "username", helpFuncCall("regex", "user.*")),
			),
			args: map[string]string{
				"key":     "the header key",
				"[value]": "the header value",
			},
		},
		validate: toKVOptionalVMatcher,
		builder: func(args any) CheckFunc {
			k, matcher := args.(*MapValueMatcher).Unpack()
			if matcher == nil {
				return func(w http.ResponseWriter, r *http.Request) bool {
					return len(r.Header[k]) > 0
				}
			}
			return func(w http.ResponseWriter, r *http.Request) bool {
				return slices.ContainsFunc(r.Header[k], matcher)
			}
		},
	},
	OnResponseHeader: {
		isResponseChecker: true,
		help: Help{
			command: OnResponseHeader,
			description: makeLines(
				"Value supports string, glob pattern, or regex pattern, e.g.:",
				helpExample(OnResponseHeader, "username", "user"),
				helpExample(OnResponseHeader, "username", helpFuncCall("glob", "user*")),
				helpExample(OnResponseHeader, "username", helpFuncCall("regex", "user.*")),
			),
			args: map[string]string{
				"key":     "the response header key",
				"[value]": "the response header value",
			},
		},
		validate: toKVOptionalVMatcher,
		builder: func(args any) CheckFunc {
			k, matcher := args.(*MapValueMatcher).Unpack()
			if matcher == nil {
				return func(w http.ResponseWriter, r *http.Request) bool {
					return len(GetInitResponseModifier(w).Header()[k]) > 0
				}
			}
			return func(w http.ResponseWriter, r *http.Request) bool {
				return slices.ContainsFunc(GetInitResponseModifier(w).Header()[k], matcher)
			}
		},
	},
	OnQuery: {
		help: Help{
			command: OnQuery,
			description: makeLines(
				"Value supports string, glob pattern, or regex pattern, e.g.:",
				helpExample(OnQuery, "username", "user"),
				helpExample(OnQuery, "username", helpFuncCall("glob", "user*")),
				helpExample(OnQuery, "username", helpFuncCall("regex", "user.*")),
			),
			args: map[string]string{
				"key":     "the query key",
				"[value]": "the query value",
			},
		},
		validate: toKVOptionalVMatcher,
		builder: func(args any) CheckFunc {
			k, matcher := args.(*MapValueMatcher).Unpack()
			if matcher == nil {
				return func(w http.ResponseWriter, r *http.Request) bool {
					return len(GetSharedData(w).GetQueries(r)[k]) > 0
				}
			}
			return func(w http.ResponseWriter, r *http.Request) bool {
				return slices.ContainsFunc(GetSharedData(w).GetQueries(r)[k], matcher)
			}
		},
	},
	OnCookie: {
		help: Help{
			command: OnCookie,
			description: makeLines(
				"Value supports string, glob pattern, or regex pattern, e.g.:",
				helpExample(OnCookie, "username", "user"),
				helpExample(OnCookie, "username", helpFuncCall("glob", "user*")),
				helpExample(OnCookie, "username", helpFuncCall("regex", "user.*")),
			),
			args: map[string]string{
				"key":     "the cookie key",
				"[value]": "the cookie value",
			},
		},
		validate: toKVOptionalVMatcher,
		builder: func(args any) CheckFunc {
			k, matcher := args.(*MapValueMatcher).Unpack()
			if matcher == nil {
				return func(w http.ResponseWriter, r *http.Request) bool {
					cookies := GetSharedData(w).GetCookies(r)
					for _, cookie := range cookies {
						if cookie.Name == k {
							return true
						}
					}
					return false
				}
			}
			return func(w http.ResponseWriter, r *http.Request) bool {
				cookies := GetSharedData(w).GetCookies(r)
				for _, cookie := range cookies {
					if cookie.Name == k {
						if matcher(cookie.Value) {
							return true
						}
					}
				}
				return false
			}
		},
	},
	OnForm: {
		help: Help{
			command: OnForm,
			description: makeLines(
				"Value supports string, glob pattern, or regex pattern, e.g.:",
				helpExample(OnForm, "username", "user"),
				helpExample(OnForm, "username", helpFuncCall("glob", "user*")),
				helpExample(OnForm, "username", helpFuncCall("regex", "user.*")),
			),
			args: map[string]string{
				"key":     "the form key",
				"[value]": "the form value",
			},
		},
		validate: toKVOptionalVMatcher,
		builder: func(args any) CheckFunc {
			k, matcher := args.(*MapValueMatcher).Unpack()
			if matcher == nil {
				return func(w http.ResponseWriter, r *http.Request) bool {
					return r.FormValue(k) != ""
				}
			}
			return func(w http.ResponseWriter, r *http.Request) bool {
				return matcher(r.FormValue(k))
			}
		},
	},
	OnPostForm: {
		help: Help{
			command: OnPostForm,
			description: makeLines(
				"Value supports string, glob pattern, or regex pattern, e.g.:",
				helpExample(OnPostForm, "username", "user"),
				helpExample(OnPostForm, "username", helpFuncCall("glob", "user*")),
				helpExample(OnPostForm, "username", helpFuncCall("regex", "user.*")),
			),
			args: map[string]string{
				"key":     "the form key",
				"[value]": "the form value",
			},
		},
		validate: toKVOptionalVMatcher,
		builder: func(args any) CheckFunc {
			k, matcher := args.(*MapValueMatcher).Unpack()
			if matcher == nil {
				return func(w http.ResponseWriter, r *http.Request) bool {
					return r.PostFormValue(k) != ""
				}
			}
			return func(w http.ResponseWriter, r *http.Request) bool {
				return matcher(r.PostFormValue(k))
			}
		},
	},
	OnMethod: {
		help: Help{
			command: OnMethod,
			args: map[string]string{
				"method": "the http method",
			},
		},
		validate: validateMethod,
		builder: func(args any) CheckFunc {
			method := args.(string)
			return func(w http.ResponseWriter, r *http.Request) bool {
				return r.Method == method
			}
		},
	},
	OnHost: {
		help: Help{
			command: OnHost,
			description: makeLines(
				"Supports string, glob pattern, or regex pattern, e.g.:",
				helpExample(OnHost, "example.com"),
				helpExample(OnHost, helpFuncCall("glob", "example*.com")),
				helpExample(OnHost, helpFuncCall("regex", `(example\w+\.com)`)),
				helpExample(OnHost, helpFuncCall("regex", `example\.com$`)),
			),
			args: map[string]string{
				"host": "the host name",
			},
		},
		validate: validateSingleMatcher,
		builder: func(args any) CheckFunc {
			matcher := args.(Matcher)
			return func(w http.ResponseWriter, r *http.Request) bool {
				return matcher(r.Host)
			}
		},
	},
	OnPath: {
		help: Help{
			command: OnPath,
			description: makeLines(
				"Supports string, glob pattern, or regex pattern, e.g.:",
				helpExample(OnPath, "/path/to"),
				helpExample(OnPath, helpFuncCall("glob", "/path/to/*")),
				helpExample(OnPath, helpFuncCall("regex", `^/path/to/.*$`)),
				helpExample(OnPath, helpFuncCall("regex", `/path/[A-Z]+/`)),
			),
			args: map[string]string{
				"path": "the request path",
			},
		},
		validate: validateURLPathMatcher,
		builder: func(args any) CheckFunc {
			matcher := args.(Matcher)
			return func(w http.ResponseWriter, r *http.Request) bool {
				reqPath := r.URL.Path
				if len(reqPath) > 0 && reqPath[0] != '/' {
					reqPath = "/" + reqPath
				}
				return matcher(reqPath)
			}
		},
	},
	OnRemote: {
		help: Help{
			command: OnRemote,
			args: map[string]string{
				"ip|cidr": "the remote ip or cidr",
			},
		},
		validate: validateCIDR,
		builder: func(args any) CheckFunc {
			ipnet := args.(*net.IPNet)
			// for /32 (IPv4) or /128 (IPv6), just compare the IP
			if ones, bits := ipnet.Mask.Size(); ones == bits {
				wantIP := ipnet.IP
				return func(w http.ResponseWriter, r *http.Request) bool {
					ip := GetSharedData(w).GetRemoteIP(r)
					if ip == nil {
						return false
					}
					return ip.Equal(wantIP)
				}
			}
			return func(w http.ResponseWriter, r *http.Request) bool {
				ip := GetSharedData(w).GetRemoteIP(r)
				if ip == nil {
					return false
				}
				return ipnet.Contains(ip)
			}
		},
	},
	OnBasicAuth: {
		help: Help{
			command: OnBasicAuth,
			args: map[string]string{
				"username": "the username",
				"password": "the password encrypted with bcrypt",
			},
		},
		validate: validateUserBCryptPassword,
		builder: func(args any) CheckFunc {
			cred := args.(*HashedCrendentials)
			return func(w http.ResponseWriter, r *http.Request) bool {
				return cred.Match(GetSharedData(w).GetBasicAuth(r))
			}
		},
	},
	OnRoute: {
		help: Help{
			command: OnRoute,
			description: makeLines(
				"Supports string, glob pattern, or regex pattern, e.g.:",
				helpExample(OnRoute, "example"),
				helpExample(OnRoute, helpFuncCall("glob", "example*")),
				helpExample(OnRoute, helpFuncCall("regex", "example\\w+")),
			),
			args: map[string]string{
				"route": "the route name",
			},
		},
		validate: validateSingleMatcher,
		builder: func(args any) CheckFunc {
			matcher := args.(Matcher)
			return func(_ http.ResponseWriter, r *http.Request) bool {
				return matcher(routes.TryGetUpstreamName(r))
			}
		},
	},
	OnStatus: {
		isResponseChecker: true,
		help: Help{
			command: OnStatus,
			description: makeLines(
				"Supported formats are:",
				helpExample(OnStatus, "<status>"),
				helpExample(OnStatus, "<status>-<status>"),
				helpExample(OnStatus, "1xx"),
				helpExample(OnStatus, "2xx"),
				helpExample(OnStatus, "3xx"),
				helpExample(OnStatus, "4xx"),
				helpExample(OnStatus, "5xx"),
			),
			args: map[string]string{
				"status": "the status code range",
			},
		},
		validate: validateStatusRange,
		builder: func(args any) CheckFunc {
			beg, end := args.(*IntTuple).Unpack()
			if beg == end {
				return func(w http.ResponseWriter, _ *http.Request) bool {
					return GetInitResponseModifier(w).StatusCode() == beg
				}
			}
			return func(w http.ResponseWriter, _ *http.Request) bool {
				statusCode := GetInitResponseModifier(w).StatusCode()
				return statusCode >= beg && statusCode <= end
			}
		},
	},
}

var (
	asciiSpace = [256]uint8{'\t': 1, '\n': 1, '\v': 1, '\f': 1, '\r': 1, ' ': 1}
	andSeps    = [256]uint8{'&': 1, '\n': 1}
)

func indexAnd(s string) int {
	for i := range s {
		if andSeps[s[i]] != 0 {
			return i
		}
	}
	return -1
}

func countAnd(s string) int {
	n := 0
	for i := range s {
		if andSeps[s[i]] != 0 {
			n++
		}
	}
	return n
}

// splitAnd splits a string by "&" and "\n" with all spaces removed.
// empty strings are not included in the result.
func splitAnd(s string) []string {
	if s == "" {
		return []string{}
	}
	n := countAnd(s)
	a := make([]string, n+1)
	i := 0
	for i < n {
		end := indexAnd(s)
		if end == -1 {
			break
		}
		beg := 0
		// trim leading spaces
		for beg < end && asciiSpace[s[beg]] != 0 {
			beg++
		}
		// trim trailing spaces
		next := end + 1
		for end-1 > beg && asciiSpace[s[end-1]] != 0 {
			end--
		}
		// skip empty segments
		if end > beg {
			a[i] = s[beg:end]
			i++
		}
		s = s[next:]
	}
	s = strings.TrimSpace(s)
	if s != "" {
		a[i] = s
		i++
	}
	return a[:i]
}

// Parse implements strutils.Parser.
func (on *RuleOn) Parse(v string) error {
	on.raw = v

	rules := splitAnd(v)
	checkAnd := make(CheckMatchAll, 0, len(rules))

	errs := gperr.NewBuilder("rule.on syntax errors")
	isResponseChecker := false
	for i, rule := range rules {
		if rule == "" {
			continue
		}
		parsed, isResp, err := parseOn(rule)
		if err != nil {
			errs.Add(err.Subjectf("line %d", i+1))
			continue
		}
		if isResp {
			isResponseChecker = true
		}
		checkAnd = append(checkAnd, parsed)
	}

	on.checker = checkAnd
	on.isResponseChecker = isResponseChecker
	return errs.Error()
}

func (on *RuleOn) String() string {
	return on.raw
}

func (on *RuleOn) MarshalText() ([]byte, error) {
	return []byte(on.String()), nil
}

func parseOn(line string) (Checker, bool, gperr.Error) {
	ors := strutils.SplitRune(line, '|')

	if len(ors) > 1 {
		errs := gperr.NewBuilder("rule.on syntax errors")
		checkOr := make(CheckMatchSingle, len(ors))
		isResponseChecker := false
		for i, or := range ors {
			curCheckers, isResp, err := parseOn(or)
			if err != nil {
				errs.Add(err)
				continue
			}
			if isResp {
				isResponseChecker = true
			}
			checkOr[i] = curCheckers.(CheckFunc)
		}
		if err := errs.Error(); err != nil {
			return nil, false, err
		}
		return checkOr, isResponseChecker, nil
	}

	subject, args, err := parse(line)
	if err != nil {
		return nil, false, err
	}

	checker, ok := checkers[subject]
	if !ok {
		return nil, false, ErrInvalidOnTarget.Subject(subject)
	}

	validArgs, err := checker.validate(args)
	if err != nil {
		return nil, false, err.Subject(subject).With(checker.help.Error())
	}

	return checker.builder(validArgs), checker.isResponseChecker, nil
}
