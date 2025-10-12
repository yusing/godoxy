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

func (on *RuleOn) Check(cached Cache, r *http.Request) bool {
	return on.checker.Check(cached, r)
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
	OnStatus = "status"
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
			description: `Value supports string, glob pattern, or regex pattern, e.g.:
			header username "user"
			header username glob("user*")
			header username regex("user.*")`,
			args: map[string]string{
				"key":     "the header key",
				"[value]": "the header value",
			},
		},
		validate: toKVOptionalVMatcher,
		builder: func(args any) CheckFunc {
			k, matcher := args.(*MapValueMatcher).Unpack()
			if matcher == nil {
				return func(cached Cache, r *http.Request) bool {
					return len(r.Header[k]) > 0
				}
			}
			return func(cached Cache, r *http.Request) bool {
				return slices.ContainsFunc(r.Header[k], matcher)
			}
		},
	},
	OnQuery: {
		help: Help{
			command: OnQuery,
			description: `Value supports string, glob pattern, or regex pattern, e.g.:
			query username "user"
			query username glob("user*")
			query username regex("user.*")`,
			args: map[string]string{
				"key":     "the query key",
				"[value]": "the query value",
			},
		},
		validate: toKVOptionalVMatcher,
		builder: func(args any) CheckFunc {
			k, matcher := args.(*MapValueMatcher).Unpack()
			if matcher == nil {
				return func(cached Cache, r *http.Request) bool {
					return len(cached.GetQueries(r)[k]) > 0
				}
			}
			return func(cached Cache, r *http.Request) bool {
				return slices.ContainsFunc(cached.GetQueries(r)[k], matcher)
			}
		},
	},
	OnCookie: {
		help: Help{
			command: OnCookie,
			description: `Value supports string, glob pattern, or regex pattern, e.g.:
			cookie username "user"
			cookie username glob("user*")
			cookie username regex("user.*")`,
			args: map[string]string{
				"key":     "the cookie key",
				"[value]": "the cookie value",
			},
		},
		validate: toKVOptionalVMatcher,
		builder: func(args any) CheckFunc {
			k, matcher := args.(*MapValueMatcher).Unpack()
			if matcher == nil {
				return func(cached Cache, r *http.Request) bool {
					cookies := cached.GetCookies(r)
					for _, cookie := range cookies {
						if cookie.Name == k {
							return true
						}
					}
					return false
				}
			}
			return func(cached Cache, r *http.Request) bool {
				cookies := cached.GetCookies(r)
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
			description: `Value supports string, glob pattern, or regex pattern, e.g.:
			form username "user"
			form username glob("user*")
			form username regex("user.*")`,
			args: map[string]string{
				"key":     "the form key",
				"[value]": "the form value",
			},
		},
		validate: toKVOptionalVMatcher,
		builder: func(args any) CheckFunc {
			k, matcher := args.(*MapValueMatcher).Unpack()
			if matcher == nil {
				return func(cached Cache, r *http.Request) bool {
					return r.FormValue(k) != ""
				}
			}
			return func(cached Cache, r *http.Request) bool {
				return matcher(r.FormValue(k))
			}
		},
	},
	OnPostForm: {
		help: Help{
			command: OnPostForm,
			description: `Value supports string, glob pattern, or regex pattern, e.g.:
			postform username "user"
			postform username glob("user*")
			postform username regex("user.*")`,
			args: map[string]string{
				"key":     "the form key",
				"[value]": "the form value",
			},
		},
		validate: toKVOptionalVMatcher,
		builder: func(args any) CheckFunc {
			k, matcher := args.(*MapValueMatcher).Unpack()
			if matcher == nil {
				return func(cached Cache, r *http.Request) bool {
					return r.PostFormValue(k) != ""
				}
			}
			return func(cached Cache, r *http.Request) bool {
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
			return func(cached Cache, r *http.Request) bool {
				return r.Method == method
			}
		},
	},
	OnHost: {
		help: Help{
			command: OnHost,
			description: `Supports string, glob pattern, or regex pattern, e.g.:
				host example.com
				host glob(example*.com)
				host regex(example\w+\.com)
				host regex(example\.com$)`,
			args: map[string]string{
				"host": "the host name",
			},
		},
		validate: validateSingleMatcher,
		builder: func(args any) CheckFunc {
			matcher := args.(Matcher)
			return func(cached Cache, r *http.Request) bool {
				return matcher(r.Host)
			}
		},
	},
	OnPath: {
		help: Help{
			command: OnPath,
			description: `Supports string, glob pattern, or regex pattern, e.g.:
				path /path/to
				path glob(/path/to/*)
				path regex(^/path/to/.*$)
				path regex(/path/[A-Z]+/)`,
			args: map[string]string{
				"path": "the request path",
			},
		},
		validate: validateURLPathMatcher,
		builder: func(args any) CheckFunc {
			matcher := args.(Matcher)
			return func(cached Cache, r *http.Request) bool {
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
				return func(cached Cache, r *http.Request) bool {
					ip := cached.GetRemoteIP(r)
					if ip == nil {
						return false
					}
					return ip.Equal(wantIP)
				}
			}
			return func(cached Cache, r *http.Request) bool {
				ip := cached.GetRemoteIP(r)
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
			return func(cached Cache, r *http.Request) bool {
				return cred.Match(cached.GetBasicAuth(r))
			}
		},
	},
	OnRoute: {
		help: Help{
			command: OnRoute,
			description: `Supports string, glob pattern, or regex pattern, e.g.:
				route example
				route glob(example*)
				route regex(example\w+)`,
			args: map[string]string{
				"route": "the route name",
			},
		},
		validate: validateSingleMatcher,
		builder: func(args any) CheckFunc {
			matcher := args.(Matcher)
			return func(_ Cache, r *http.Request) bool {
				return matcher(routes.TryGetUpstreamName(r))
			}
		},
	},
	OnStatus: {
		help: Help{
			command: OnStatus,
			description: `Supported formats are:
				- <status>
				- <status>-<status>
				- 1xx
				- 2xx
				- 3xx
				- 4xx
				- 5xx`,
			args: map[string]string{
				"status": "the status code range",
			},
		},
		validate: validateStatusRange,
		builder: func(args any) CheckFunc {
			beg, end := args.(*IntTuple).Unpack()
			if beg == end {
				return func(cached Cache, r *http.Request) bool {
					return r.Response.StatusCode == beg
				}
			}
			return func(cached Cache, r *http.Request) bool {
				return r.Response.StatusCode >= beg && r.Response.StatusCode <= end
			}
		},
		isResponseChecker: true,
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
		return nil, false, err.Subject(subject).Withf("%s", checker.help.String())
	}

	return checker.builder(validArgs), checker.isResponseChecker, nil
}
