package rules

import (
	"net"
	"net/http"
	"slices"
	"strings"

	"github.com/yusing/godoxy/internal/route/routes"
	gperr "github.com/yusing/goutils/errs"
	httputils "github.com/yusing/goutils/http"
)

type RuleOn struct {
	raw     string
	checker Checker
	phase   PhaseFlag
}

func (on *RuleOn) Check(w http.ResponseWriter, r *http.Request) bool {
	if on.checker == nil {
		return true
	}
	return on.checker.Check(httputils.GetInitResponseModifier(w), r)
}

// on request
const (
	OnDefault   = "default"
	OnHeader    = "header"
	OnQuery     = "query"
	OnCookie    = "cookie"
	OnForm      = "form"
	OnPostForm  = "postform"
	OnProto     = "proto"
	OnMethod    = "method"
	OnHost      = "host"
	OnPath      = "path"
	OnRemote    = "remote"
	OnBasicAuth = "basic_auth"
	OnRoute     = "route"
)

// on response
const (
	OnResponseHeader = "resp_header"
	OnStatus         = "status"
)

var checkers = map[string]struct {
	help     Help
	validate ValidateFunc
	builder  func(args any) CheckFunc
}{
	OnDefault: {
		help: Help{
			command: OnDefault,
			description: makeLines(
				"Select the default (fallback) rule.",
			),
			args: map[string]string{},
		},
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			if len(args) != 0 {
				return phase, nil, ErrExpectNoArg
			}
			return phase, nil, nil
		},
		builder: func(args any) CheckFunc {
			return func(w *httputils.ResponseModifier, r *http.Request) bool { return true }
		},
	},
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
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			parsedArgs, err = toKVOptionalVMatcher(args)
			return
		},
		builder: func(args any) CheckFunc {
			k, matcher := args.(*MapValueMatcher).Unpack()
			if matcher == nil {
				return func(w *httputils.ResponseModifier, r *http.Request) bool {
					return len(r.Header[k]) > 0
				}
			}
			return func(w *httputils.ResponseModifier, r *http.Request) bool {
				return slices.ContainsFunc(r.Header[k], matcher)
			}
		},
	},
	OnResponseHeader: {
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
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			phase = PhasePost
			parsedArgs, err = toKVOptionalVMatcher(args)
			return
		},
		builder: func(args any) CheckFunc {
			k, matcher := args.(*MapValueMatcher).Unpack()
			if matcher == nil {
				return func(w *httputils.ResponseModifier, r *http.Request) bool {
					return len(w.Header()[k]) > 0
				}
			}
			return func(w *httputils.ResponseModifier, r *http.Request) bool {
				return slices.ContainsFunc(w.Header()[k], matcher)
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
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			parsedArgs, err = toKVOptionalVMatcher(args)
			return
		},
		builder: func(args any) CheckFunc {
			k, matcher := args.(*MapValueMatcher).Unpack()
			if matcher == nil {
				return func(w *httputils.ResponseModifier, r *http.Request) bool {
					return len(w.SharedData().GetQueries(r)[k]) > 0
				}
			}
			return func(w *httputils.ResponseModifier, r *http.Request) bool {
				return slices.ContainsFunc(w.SharedData().GetQueries(r)[k], matcher)
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
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			parsedArgs, err = toKVOptionalVMatcher(args)
			return
		},
		builder: func(args any) CheckFunc {
			k, matcher := args.(*MapValueMatcher).Unpack()
			if matcher == nil {
				return func(w *httputils.ResponseModifier, r *http.Request) bool {
					cookies := w.SharedData().GetCookies(r)
					for _, cookie := range cookies {
						if cookie.Name == k {
							return true
						}
					}
					return false
				}
			}
			return func(w *httputils.ResponseModifier, r *http.Request) bool {
				cookies := w.SharedData().GetCookies(r)
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
	//nolint:dupl
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
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			parsedArgs, err = toKVOptionalVMatcher(args)
			return
		},
		builder: func(args any) CheckFunc {
			k, matcher := args.(*MapValueMatcher).Unpack()
			if matcher == nil {
				return func(w *httputils.ResponseModifier, r *http.Request) bool {
					return r.FormValue(k) != ""
				}
			}
			return func(w *httputils.ResponseModifier, r *http.Request) bool {
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
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			parsedArgs, err = toKVOptionalVMatcher(args)
			return
		},
		builder: func(args any) CheckFunc {
			k, matcher := args.(*MapValueMatcher).Unpack()
			if matcher == nil {
				return func(w *httputils.ResponseModifier, r *http.Request) bool {
					return r.PostFormValue(k) != ""
				}
			}
			return func(w *httputils.ResponseModifier, r *http.Request) bool {
				return matcher(r.PostFormValue(k))
			}
		},
	},
	OnProto: {
		help: Help{
			command: OnProto,
			description: makeLines(
				"Match inbound scheme or protocol family.",
				"http/https match cleartext vs TLS regardless of HTTP version.",
				"h1 matches cleartext HTTP/1.x; h2 matches TLS HTTP/2; h2c matches cleartext HTTP/2; h3 matches TLS HTTP/3.",
				helpExample(OnProto, "https"),
				helpExample(OnProto, "h2"),
			),
			args: map[string]string{
				"proto": "http, https, h1 (cleartext HTTP/1.x), h2 (TLS HTTP/2), h2c (cleartext HTTP/2), or h3 (TLS HTTP/3)",
			},
		},
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			if len(args) != 1 {
				return phase, nil, ErrExpectOneArg
			}
			proto := args[0]
			switch proto {
			case "http", "https", "h1", "h2", "h2c", "h3":
				return phase, proto, nil
			default:
				return phase, nil, ErrInvalidArguments.Withf("proto: %q", proto)
			}
		},
		builder: func(args any) CheckFunc {
			proto := args.(string)
			switch proto {
			case "http":
				return func(w *httputils.ResponseModifier, r *http.Request) bool {
					return r.TLS == nil
				}
			case "https":
				return func(w *httputils.ResponseModifier, r *http.Request) bool {
					return r.TLS != nil
				}
			case "h1":
				return func(w *httputils.ResponseModifier, r *http.Request) bool {
					return r.TLS == nil && r.ProtoMajor == 1
				}
			case "h2":
				return func(w *httputils.ResponseModifier, r *http.Request) bool {
					return r.TLS != nil && r.ProtoMajor == 2
				}
			case "h2c":
				return func(w *httputils.ResponseModifier, r *http.Request) bool {
					return r.TLS == nil && r.ProtoMajor == 2
				}
			default: // h3
				return func(w *httputils.ResponseModifier, r *http.Request) bool {
					return r.TLS != nil && r.ProtoMajor == 3
				}
			}
		},
	},
	OnMethod: {
		help: Help{
			command: OnMethod,
			description: makeLines(
				"Match inbound HTTP method (verb).",
				helpExample(OnMethod, "GET"),
				helpExample(OnMethod, "OPTIONS"),
			),
			args: map[string]string{
				"method": "canonical method name such as GET, HEAD, POST, PUT, PATCH, DELETE, CONNECT, OPTIONS, TRACE",
			},
		},
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			parsedArgs, err = validateMethod(args)
			return
		},
		builder: func(args any) CheckFunc {
			method := args.(string)
			return func(w *httputils.ResponseModifier, r *http.Request) bool {
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
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			parsedArgs, err = validateSingleMatcher(args)
			return
		},
		builder: func(args any) CheckFunc {
			matcher := args.(Matcher)
			return func(w *httputils.ResponseModifier, r *http.Request) bool {
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
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			parsedArgs, err = validateURLPathMatcher(args)
			return
		},
		builder: func(args any) CheckFunc {
			matcher := args.(Matcher)
			return func(w *httputils.ResponseModifier, r *http.Request) bool {
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
			description: makeLines(
				"Match remote client IP against an address or CIDR.",
				"Bare IPv4 input is treated as /32 for an exact host match.",
				"Use /128 for an exact IPv6 host; bare IPv6 input expands to /32, and wider prefixes match the whole subnet.",
				helpExample(OnRemote, "203.0.113.42"),
				helpExample(OnRemote, "192.168.0.0/16"),
				helpExample(OnRemote, "2001:db8::1/128"),
				helpExample(OnRemote, "2001:db8::/32"),
			),
			args: map[string]string{
				"ip|cidr": "IPv4/IPv6 CIDR, or a bare IPv4 address for /32 exact match",
			},
		},
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			parsedArgs, err = validateCIDR(args)
			return
		},
		builder: func(args any) CheckFunc {
			ipnet := args.(*net.IPNet)
			// for /32 (IPv4) or /128 (IPv6), just compare the IP
			if ones, bits := ipnet.Mask.Size(); ones == bits {
				wantIP := ipnet.IP
				return func(w *httputils.ResponseModifier, r *http.Request) bool {
					ip := w.SharedData().GetRemoteIP(r)
					if ip == nil {
						return false
					}
					return ip.Equal(wantIP)
				}
			}
			return func(w *httputils.ResponseModifier, r *http.Request) bool {
				ip := w.SharedData().GetRemoteIP(r)
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
			description: makeLines(
				"Match HTTP Basic Authorization on the inbound request.",
				"Decoded credentials must match the username and bcrypt password hash configured in the rule.",
				helpExample(OnBasicAuth, "<user>", "<bcrypt-hash>"),
			),
			args: map[string]string{
				"username": "expected plain-text username",
				"password": "bcrypt cost hash corresponding to that user",
			},
		},
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			parsedArgs, err = validateUserBCryptPassword(args)
			return
		},
		builder: func(args any) CheckFunc {
			cred := args.(*HashedCrendentials)
			return func(w *httputils.ResponseModifier, r *http.Request) bool {
				return cred.Match(w.SharedData().GetBasicAuth(r))
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
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			parsedArgs, err = validateSingleMatcher(args)
			return
		},
		builder: func(args any) CheckFunc {
			matcher := args.(Matcher)
			return func(w *httputils.ResponseModifier, r *http.Request) bool {
				return matcher(routes.TryGetUpstreamName(r))
			}
		},
	},
	OnStatus: {
		help: Help{
			command: OnStatus,
			description: makeLines(
				"Match current post-phase response status (exact, range, or class).",
				"Evaluated after upstream responds; earlier post-phase rewrites can change the status seen here.",
				helpExample(OnStatus, "<status>"),
				helpExample(OnStatus, "<status>-<status>"),
				helpExample(OnStatus, "1xx"),
				helpExample(OnStatus, "2xx"),
				helpExample(OnStatus, "3xx"),
				helpExample(OnStatus, "4xx"),
				helpExample(OnStatus, "5xx"),
			),
			args: map[string]string{
				"status": "code (404), inclusive range (502-504), or class (4xx)",
			},
		},
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			phase = PhasePost
			parsedArgs, err = validateStatusRange(args)
			return
		},
		builder: func(args any) CheckFunc {
			beg, end := args.(*IntTuple).Unpack()
			if beg == end {
				return func(w *httputils.ResponseModifier, _ *http.Request) bool {
					return w.StatusCode() == beg
				}
			}
			return func(w *httputils.ResponseModifier, _ *http.Request) bool {
				statusCode := w.StatusCode()
				return statusCode >= beg && statusCode <= end
			}
		},
	},
}

var (
	asciiSpace = [256]uint8{'\t': 1, '\n': 1, '\v': 1, '\f': 1, '\r': 1, ' ': 1}
	andSeps    = [256]uint8{'&': 1, '\n': 1}
)

// splitAnd splits a condition string into AND parts.
// It treats '&' and newline as AND separators, except when a line ends with
// an unescaped '|' (OR continuation), where the newline stays in the same part.
// Empty parts are omitted.
func splitAnd(s string) []string {
	if s == "" {
		return []string{}
	}
	result := []string{}
	forEachAndPart(s, func(part string) {
		result = append(result, part)
	})
	return result
}

func lineEndsWithUnescapedPipe(s string, start, end int) bool {
	for i := end - 1; i >= start; i-- {
		if asciiSpace[s[i]] != 0 {
			continue
		}
		if s[i] != '|' {
			return false
		}
		escapes := 0
		for j := i - 1; j >= start && s[j] == '\\'; j-- {
			escapes++
		}
		return escapes%2 == 0
	}
	return false
}

func advanceSplitState(s string, i *int, quote *byte, brackets *int) bool {
	c := s[*i]
	if *quote != 0 {
		if c == '\\' && *i+1 < len(s) {
			*i++
			return true
		}
		if c == *quote {
			*quote = 0
		}
		return true
	}

	switch c {
	case '\\':
		if *i+1 < len(s) {
			*i++
			return true
		}
	case '"', '\'', '`':
		*quote = c
		return true
	case '(':
		*brackets++
		return true
	case ')':
		if *brackets > 0 {
			*brackets--
		}
		return true
	}
	return false
}

// splitPipe splits a string by "|" but respects quotes, brackets, and escaped characters.
// It's similar to the parser.go logic but specifically for pipe splitting.
func splitPipe(s string) []string {
	if s == "" {
		return []string{}
	}

	result := []string{}
	forEachPipePart(s, func(part string) {
		result = append(result, part)
	})
	return result
}

func forEachAndPart(s string, fn func(part string)) {
	quote := byte(0)
	brackets := 0
	start := 0

	for i := 0; i <= len(s); i++ {
		if i < len(s) {
			c := s[i]
			if advanceSplitState(s, &i, &quote, &brackets) {
				continue
			}

			if c == '\n' {
				if brackets > 0 || lineEndsWithUnescapedPipe(s, start, i) {
					continue
				}
			} else if c != '&' || brackets > 0 {
				continue
			}
		}

		if i < len(s) && andSeps[s[i]] == 0 {
			continue
		}
		part := strings.TrimSpace(s[start:i])
		if part != "" {
			fn(part)
		}
		start = i + 1
	}
}

func forEachPipePart(s string, fn func(part string)) {
	quote := byte(0)
	brackets := 0
	start := 0

	for i := 0; i < len(s); i++ {
		if advanceSplitState(s, &i, &quote, &brackets) {
			continue
		}
		if s[i] == '|' && brackets == 0 {
			if part := strings.TrimSpace(s[start:i]); part != "" {
				fn(part)
			}
			start = i + 1
		}
	}
	if start < len(s) {
		if part := strings.TrimSpace(s[start:]); part != "" {
			fn(part)
		}
	}
}

// Parse implements strutils.Parser.
func (on *RuleOn) Parse(v string) error {
	on.raw = v

	ruleCount := 0
	forEachAndPart(v, func(_ string) {
		ruleCount++
	})
	checkAnd := make(CheckMatchAll, 0, ruleCount)

	errs := gperr.NewBuilder("rule.on syntax errors")
	i := 0
	forEachAndPart(v, func(rule string) {
		i++
		parsed, phase, err := parseOn(rule)
		if err != nil {
			errs.AddSubjectf(err, "line %d", i)
			return
		}
		on.phase |= phase
		checkAnd = append(checkAnd, parsed)
	})

	on.checker = checkAnd
	return errs.Error()
}

func (on *RuleOn) String() string {
	return on.raw
}

func (on *RuleOn) MarshalText() ([]byte, error) {
	return []byte(on.String()), nil
}

func parseOn(line string) (Checker, PhaseFlag, error) {
	orCount := 0
	forEachPipePart(line, func(_ string) {
		orCount++
	})
	if orCount > 1 {
		var phase PhaseFlag
		errs := gperr.NewBuilder("rule.on syntax errors")
		checkOr := make(CheckMatchSingle, orCount)
		i := 0
		forEachPipePart(line, func(or string) {
			i++
			checkFunc, req, err := parseOnAtom(or)
			if err != nil {
				errs.AddSubjectf(err, "or[%d]", i)
				return
			}
			checkOr[i-1] = checkFunc
			phase |= req
		})
		if err := errs.Error(); err != nil {
			return nil, phase, err
		}
		return checkOr, phase, nil
	}

	return parseOnAtom(line)
}

func parseOnAtom(line string) (CheckFunc, PhaseFlag, error) {
	var phase PhaseFlag
	subject, args, err := parse(line)
	if err != nil {
		return nil, phase, err
	}

	negate := false
	if strings.HasPrefix(subject, "!") {
		negate = true
		subject = subject[1:]
	}

	checker, ok := checkers[subject]
	if !ok {
		return nil, phase, ErrInvalidOnTarget.Subject(subject)
	}

	req, validArgs, err := checker.validate(args)
	if err != nil {
		return nil, phase, gperr.Wrap(err).With(checker.help.Error())
	}
	phase |= req

	checkFunc := checker.builder(validArgs)
	if negate {
		origCheckFunc := checkFunc
		checkFunc = func(w *httputils.ResponseModifier, r *http.Request) bool {
			return !origCheckFunc(w, r)
		}
	}
	return checkFunc, phase, nil
}
