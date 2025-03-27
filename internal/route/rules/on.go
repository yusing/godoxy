package rules

import (
	"net/http"

	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

type RuleOn struct {
	raw     string
	checker Checker
}

const (
	OnHeader    = "header"
	OnQuery     = "query"
	OnCookie    = "cookie"
	OnForm      = "form"
	OnPostForm  = "postform"
	OnMethod    = "method"
	OnPath      = "path"
	OnRemote    = "remote"
	OnBasicAuth = "basic_auth"
)

var checkers = map[string]struct {
	help     Help
	validate ValidateFunc
	builder  func(args any) CheckFunc
}{
	OnHeader: {
		help: Help{
			command: OnHeader,
			args: map[string]string{
				"key":     "the header key",
				"[value]": "the header value",
			},
		},
		validate: toKVOptionalV,
		builder: func(args any) CheckFunc {
			k, v := args.(*StrTuple).Unpack()
			if v == "" {
				return func(cached Cache, r *http.Request) bool {
					return len(r.Header[k]) > 0
				}
			}
			return func(cached Cache, r *http.Request) bool {
				for _, vv := range r.Header[k] {
					if v == vv {
						return true
					}
				}
				return false
			}
		},
	},
	OnQuery: {
		help: Help{
			command: OnQuery,
			args: map[string]string{
				"key":     "the query key",
				"[value]": "the query value",
			},
		},
		validate: toKVOptionalV,
		builder: func(args any) CheckFunc {
			k, v := args.(*StrTuple).Unpack()
			if v == "" {
				return func(cached Cache, r *http.Request) bool {
					return len(cached.GetQueries(r)[k]) > 0
				}
			}
			return func(cached Cache, r *http.Request) bool {
				queries := cached.GetQueries(r)[k]
				for _, query := range queries {
					if query == v {
						return true
					}
				}
				return false
			}
		},
	},
	OnCookie: {
		help: Help{
			command: OnCookie,
			args: map[string]string{
				"key":     "the cookie key",
				"[value]": "the cookie value",
			},
		},
		validate: toKVOptionalV,
		builder: func(args any) CheckFunc {
			k, v := args.(*StrTuple).Unpack()
			if v == "" {
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
					if cookie.Name == k &&
						cookie.Value == v {
						return true
					}
				}
				return false
			}
		},
	},
	OnForm: {
		help: Help{
			command: OnForm,
			args: map[string]string{
				"key":     "the form key",
				"[value]": "the form value",
			},
		},
		validate: toKVOptionalV,
		builder: func(args any) CheckFunc {
			k, v := args.(*StrTuple).Unpack()
			if v == "" {
				return func(cached Cache, r *http.Request) bool {
					return r.FormValue(k) != ""
				}
			}
			return func(cached Cache, r *http.Request) bool {
				return r.FormValue(k) == v
			}
		},
	},
	OnPostForm: {
		help: Help{
			command: OnPostForm,
			args: map[string]string{
				"key":     "the form key",
				"[value]": "the form value",
			},
		},
		validate: toKVOptionalV,
		builder: func(args any) CheckFunc {
			k, v := args.(*StrTuple).Unpack()
			if v == "" {
				return func(cached Cache, r *http.Request) bool {
					return r.PostFormValue(k) != ""
				}
			}
			return func(cached Cache, r *http.Request) bool {
				return r.PostFormValue(k) == v
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
	OnPath: {
		help: Help{
			command: OnPath,
			description: `The path can be a glob pattern, e.g.:
				/path/to
				/path/to/*`,
			args: map[string]string{
				"path": "the request path",
			},
		},
		validate: validateURLPath,
		builder: func(args any) CheckFunc {
			pat := args.(string)
			return func(cached Cache, r *http.Request) bool {
				reqPath := r.URL.Path
				if len(reqPath) > 0 && reqPath[0] != '/' {
					reqPath = "/" + reqPath
				}
				return strutils.GlobMatch(pat, reqPath)
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
			cidr := args.(types.CIDR)
			return func(cached Cache, r *http.Request) bool {
				ip := cached.GetRemoteIP(r)
				if ip == nil {
					return false
				}
				return cidr.Contains(ip)
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
}

// Parse implements strutils.Parser.
func (on *RuleOn) Parse(v string) error {
	on.raw = v

	lines := strutils.SplitLine(v)
	checkAnd := make(CheckMatchAll, 0, len(lines))

	errs := gperr.NewBuilder("rule.on syntax errors")
	for i, line := range lines {
		if line == "" {
			continue
		}
		parsed, err := parseOn(line)
		if err != nil {
			errs.Add(err.Subjectf("line %d", i+1))
			continue
		}
		checkAnd = append(checkAnd, parsed)
	}

	on.checker = checkAnd
	return errs.Error()
}

func (on *RuleOn) String() string {
	return on.raw
}

func (on *RuleOn) MarshalText() ([]byte, error) {
	return []byte(on.String()), nil
}

func parseOn(line string) (Checker, gperr.Error) {
	ors := strutils.SplitRune(line, '|')

	if len(ors) > 1 {
		errs := gperr.NewBuilder("rule.on syntax errors")
		checkOr := make(CheckMatchSingle, len(ors))
		for i, or := range ors {
			curCheckers, err := parseOn(or)
			if err != nil {
				errs.Add(err)
				continue
			}
			checkOr[i] = curCheckers.(CheckFunc)
		}
		if err := errs.Error(); err != nil {
			return nil, err
		}
		return checkOr, nil
	}

	subject, args, err := parse(line)
	if err != nil {
		return nil, err
	}

	checker, ok := checkers[subject]
	if !ok {
		return nil, ErrInvalidOnTarget.Subject(subject)
	}

	validArgs, err := checker.validate(args)
	if err != nil {
		return nil, err.Subject(subject).Withf("%s", checker.help.String())
	}

	return checker.builder(validArgs), nil
}
