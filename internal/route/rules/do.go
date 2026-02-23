package rules

import (
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
	entrypoint "github.com/yusing/godoxy/internal/entrypoint/types"
	"github.com/yusing/godoxy/internal/logging"
	gphttp "github.com/yusing/godoxy/internal/net/gphttp"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/notif"
	"github.com/yusing/godoxy/internal/route/routes"
	"github.com/yusing/godoxy/internal/types"
	gperr "github.com/yusing/goutils/errs"
	httputils "github.com/yusing/goutils/http"
	"github.com/yusing/goutils/http/reverseproxy"
)

type (
	Command struct {
		raw  string
		pre  Commands // runs before w.WriteHeader
		post Commands
	}
)

const (
	CommandUpstream     = "upstream"
	CommandUpstreamOld  = "bypass"
	CommandUpstreamOld2 = "pass"

	CommandRequireAuth      = "require_auth"
	CommandRewrite          = "rewrite"
	CommandServe            = "serve"
	CommandProxy            = "proxy"
	CommandRedirect         = "redirect"
	CommandRoute            = "route"
	CommandError            = "error"
	CommandRequireBasicAuth = "require_basic_auth"
	CommandSet              = "set"
	CommandAdd              = "add"
	CommandRemove           = "remove"
	CommandLog              = "log"
	CommandNotify           = "notify"
)

type AuthHandler func(w http.ResponseWriter, r *http.Request) (proceed bool)

var authHandler AuthHandler

func InitAuthHandler(handler AuthHandler) {
	authHandler = handler
}

func init() {
	commands[CommandUpstreamOld] = commands[CommandUpstream]
	commands[CommandUpstreamOld2] = commands[CommandUpstream]
}

var commands = map[string]struct {
	help      Help
	validate  ValidateFunc
	build     func(args any) HandlerFunc
	terminate bool
}{
	CommandUpstream: {
		help: Help{
			command:     CommandUpstream,
			description: makeLines("Pass the request to the upstream"),
			args:        map[string]string{},
		},
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			if len(args) != 0 {
				return phase, nil, ErrExpectNoArg
			}
			return phase, nil, nil
		},
		build: func(args any) HandlerFunc {
			return func(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
				upstream(w, r)
				return errTerminateRule
			}
		},
		terminate: true,
	},
	CommandRequireAuth: {
		help: Help{
			command:     CommandRequireAuth,
			description: makeLines("Require HTTP authentication for incoming requests"),
			args:        map[string]string{},
		},
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			phase = PhasePre
			if len(args) != 0 {
				return phase, nil, ErrExpectNoArg
			}
			return phase, nil, nil
		},
		build: func(args any) HandlerFunc {
			return func(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
				if authHandler == nil { // no auth handler configured, allow request to proceed
					return nil
				}
				if proceed := authHandler(w, r); !proceed {
					return errTerminateRule
				}
				return nil
			}
		},
	},
	CommandRewrite: {
		help: Help{
			command: CommandRewrite,
			description: makeLines(
				"Rewrite a request path from one prefix to another, e.g.:",
				helpExample(CommandRewrite, "/foo", "/bar"),
			),
			args: map[string]string{
				"from": "the path to rewrite, must start with /",
				"to":   "the path to rewrite to, must start with /",
			},
		},
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			phase = PhasePre
			if len(args) != 2 {
				return phase, nil, ErrExpectTwoArgs
			}
			path1, err1 := validateURLPath(args[:1])
			path2, err2 := validateURLPath(args[1:])
			if err1 != nil {
				err1 = gperr.Errorf("from: %w", err1)
			}
			if err2 != nil {
				err2 = gperr.Errorf("to: %w", err2)
			}
			if err1 != nil || err2 != nil {
				return phase, nil, gperr.Join(err1, err2)
			}
			return phase, &StrTuple{path1.(string), path2.(string)}, nil
		},
		build: func(args any) HandlerFunc {
			orig, repl := args.(*StrTuple).Unpack()
			return func(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
				path := r.URL.Path
				if len(path) > 0 && path[0] != '/' {
					path = "/" + path
				}
				if !strings.HasPrefix(path, orig) {
					return nil
				}
				path = repl + path[len(orig):]
				r.URL.Path = path
				r.URL.RawPath = ""
				r.RequestURI = ""
				return nil
			}
		},
	},
	CommandServe: {
		help: Help{
			command: CommandServe,
			description: makeLines(
				"Serve static files from a local file system path, e.g.:",
				helpExample(CommandServe, "/var/www"),
			),
			args: map[string]string{
				"root": "the file system path to serve, must be an existing directory",
			},
		},
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			phase = PhasePre
			parsedArgs, err = validateFSPath(args)
			return
		},
		build: func(args any) HandlerFunc {
			root := args.(string)
			return func(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
				http.ServeFile(w, r, path.Join(root, path.Clean(r.URL.Path)))
				return errTerminateRule
			}
		},
		terminate: true,
	},
	CommandRedirect: {
		help: Help{
			command: CommandRedirect,
			description: makeLines(
				"Redirect request to another URL, e.g.:",
				helpExample(CommandRedirect, "https://example.com"),
			),
			args: map[string]string{
				"to": "the url to redirect to, can be relative or absolute URL",
			},
		},
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			phase = PhasePre
			parsedArgs, err = validateURL(args)
			return
		},
		build: func(args any) HandlerFunc {
			target := args.(*nettypes.URL).String()
			return func(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
				http.Redirect(w, r, target, http.StatusTemporaryRedirect)
				return errTerminateRule
			}
		},
		terminate: true,
	},
	CommandRoute: {
		help: Help{
			command: CommandRoute,
			description: makeLines(
				"Route the request to another route, e.g.:",
				helpExample(CommandRoute, "route1"),
			),
			args: map[string]string{
				"route": "the route to route to",
			},
		},
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			phase = PhasePre
			if len(args) != 1 {
				return phase, nil, ErrExpectOneArg
			}
			return phase, args[0], nil
		},
		build: func(args any) HandlerFunc {
			route := args.(string)
			return func(w *httputils.ResponseModifier, req *http.Request, upstream http.HandlerFunc) error {
				ep := entrypoint.FromCtx(req.Context())
				r, ok := ep.HTTPRoutes().Get(route)
				if !ok {
					excluded, has := ep.ExcludedRoutes().Get(route)
					if has {
						r, ok = excluded.(types.HTTPRoute)
					}
				}
				if ok {
					r.ServeHTTP(w, req)
				} else {
					http.Error(w, fmt.Sprintf("Route %q not found", route), http.StatusNotFound)
				}
				return errTerminateRule
			}
		},
		terminate: true,
	},
	CommandError: {
		help: Help{
			command: CommandError,
			description: makeLines(
				"Send an HTTP error response and terminate processing, e.g.:",
				helpExample(CommandError, "400", "bad request"),
			),
			args: map[string]string{
				"code": "the http status code to return",
				"text": "the error message to return",
			},
		},
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			phase = PhasePre
			if len(args) != 2 {
				return phase, nil, ErrExpectTwoArgs
			}
			codeStr, text := args[0], args[1]
			code, err := strconv.Atoi(codeStr)
			if err != nil {
				return phase, nil, ErrInvalidArguments.With(err)
			}
			if !httputils.IsStatusCodeValid(code) {
				return phase, nil, ErrInvalidArguments.Subject(codeStr)
			}
			tmplReq, textTmpl, err := validateTemplate(text, true)
			if err != nil {
				return phase, nil, ErrInvalidArguments.With(err)
			}
			phase |= tmplReq
			return phase, &Tuple[int, templateString]{code, textTmpl}, nil
		},
		build: func(args any) HandlerFunc {
			code, textTmpl := args.(*Tuple[int, templateString]).Unpack()
			return func(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
				// error command should overwrite the response body
				w.ResetBody()
				w.WriteHeader(code)
				_, err := textTmpl.ExpandVars(w, r, w.BodyBuffer())
				if err != nil {
					return err
				}
				return errTerminateRule
			}
		},
		terminate: true,
	},
	CommandRequireBasicAuth: {
		help: Help{
			command: CommandRequireBasicAuth,
			description: makeLines(
				"Require HTTP basic authentication for incoming requests, e.g.:",
				helpExample(CommandRequireBasicAuth, "Restricted Area"),
			),
			args: map[string]string{
				"realm": "the authentication realm",
			},
		},
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			phase = PhasePre
			if len(args) == 1 {
				return phase, args[0], nil
			}
			return phase, nil, ErrExpectOneArg
		},
		build: func(args any) HandlerFunc {
			realm := args.(string)
			return func(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
				w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Basic realm=%q`, realm))
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return errTerminateRule
			}
		},
		terminate: true,
	},
	CommandProxy: {
		help: Help{
			command: CommandProxy,
			description: makeLines(
				"Proxy the request to the specified absolute URL, e.g.:",
				helpExample(CommandProxy, "http://upstream:8080"),
			),
			args: map[string]string{
				"to": "the url to proxy to, must be an absolute URL",
			},
		},
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			phase = PhasePre
			parsedArgs, err = validateURL(args)
			return
		},
		build: func(args any) HandlerFunc {
			target := args.(*nettypes.URL)
			if target.Scheme == "" {
				target.Scheme = "http"
			}
			if target.Host == "" {
				rawPath := target.EscapedPath()
				return func(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
					url := target.URL
					url.Host = routes.TryGetUpstreamHostPort(r)
					if url.Host == "" {
						return fmt.Errorf("no upstream host: %s", r.URL.String())
					}
					rp := reverseproxy.NewReverseProxy(url.Host, &url, gphttp.NewTransport())
					r.URL.Path = target.Path
					r.URL.RawPath = rawPath
					r.RequestURI = ""
					rp.ServeHTTP(w, r)
					return errTerminateRule
				}
			}
			rp := reverseproxy.NewReverseProxy("", &target.URL, gphttp.NewTransport())
			return func(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
				rp.ServeHTTP(w, r)
				return errTerminateRule
			}
		},
		terminate: true,
	},
	CommandSet: {
		help: Help{
			command: CommandSet,
			description: makeLines(
				"Set a field in the request or response, e.g.:",
				helpExample(CommandSet, "header", "User-Agent", "godoxy"),
			),
			args: map[string]string{
				"target": "the target to set, can be " + strings.Join(AllFields, ", "),
				"field":  "the field to set",
				"value":  "the value to set",
			},
		},
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			return validateModField(ModFieldSet, args)
		},
		build: func(args any) HandlerFunc {
			return args.(HandlerFunc)
		},
	},
	CommandAdd: {
		help: Help{
			command: CommandAdd,
			description: makeLines(
				"Add a value to a field in the request or response, e.g.:",
				helpExample(CommandAdd, "header", "X-Foo", "bar"),
			),
			args: map[string]string{
				"target": "the target to add, can be " + strings.Join(AllFields, ", "),
				"field":  "the field to add",
				"value":  "the value to add",
			},
		},
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			return validateModField(ModFieldAdd, args)
		},
		build: func(args any) HandlerFunc {
			return args.(HandlerFunc)
		},
	},
	CommandRemove: {
		help: Help{
			command: CommandRemove,
			description: makeLines(
				"Remove a field from the request or response, e.g.:",
				helpExample(CommandRemove, "header", "User-Agent"),
			),
			args: map[string]string{
				"target": "the target to remove, can be " + strings.Join(AllFields, ", "),
				"field":  "the field to remove",
			},
		},
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			return validateModField(ModFieldRemove, args)
		},
		build: func(args any) HandlerFunc {
			return args.(HandlerFunc)
		},
	},
	CommandLog: {
		help: Help{
			command: CommandLog,
			description: makeLines(
				"The template supports the following variables:",
				helpListItem("Request", "the request object"),
				helpListItem("Response", "the response object"),
				"",
				"Example:",
				helpExample(CommandLog, "info", "/dev/stdout", "$req_method $req_url $status_code"),
			),
			args: map[string]string{
				"level":    "the log level",
				"path":     "the log path (/dev/stdout for stdout, /dev/stderr for stderr)",
				"template": "the template to log",
			},
		},
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			if len(args) != 3 {
				return phase, nil, ErrExpectThreeArgs
			}
			phase, tmpl, err := validateTemplate(args[2], true)
			if err != nil {
				return phase, nil, err
			}
			level, err := validateLevel(args[0])
			if err != nil {
				return phase, nil, err
			}
			// NOTE: file will stay opened forever
			// it leverages accesslog.NewFileIO so
			// it will be opened only once for the same path
			f, err := openFile(args[1])
			if err != nil {
				return phase, nil, err
			}
			return phase, &onLogArgs{level, f, tmpl}, nil
		},
		build: func(args any) HandlerFunc {
			level, f, tmpl := args.(*onLogArgs).Unpack()
			var logger io.Writer
			isStdLogger := f == stdout || f == stderr
			if isStdLogger {
				logger = logging.NewLoggerWithFixedLevel(level, f)
			} else {
				logger = f
			}
			return func(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
				if isStdLogger {
					bufPool := w.BufPool()
					buf := bufPool.GetBuffer()
					defer bufPool.PutBuffer(buf)

					if _, err := tmpl.ExpandVars(w, r, buf); err != nil {
						return err
					}
					if buf.Len() == 0 {
						return nil
					}
					_, err := logger.Write(buf.Bytes())
					return err
				}
				_, err := tmpl.ExpandVars(w, r, logger)
				return err
			}
		},
	},
	CommandNotify: {
		help: Help{
			command: CommandNotify,
			description: makeLines(
				"The template supports the following variables:",
				helpListItem("Request", "the request object"),
				helpListItem("Response", "the response object"),
				"",
				"Example:",
				helpExample(CommandNotify, "info", "ntfy", "Received request to $req_url", "$req_method $status_code"),
			),
			args: map[string]string{
				"level":    "the log level",
				"provider": "the notification provider (must be defined in config `providers.notification`)",
				"title":    "the title of the notification",
				"body":     "the body of the notification",
			},
		},
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			if len(args) != 4 {
				return phase, nil, ErrExpectFourArgs
			}
			req1, titleTmpl, err := validateTemplate(args[2], false)
			if err != nil {
				return phase, nil, err
			}
			req2, bodyTmpl, err := validateTemplate(args[3], false)
			if err != nil {
				return phase, nil, err
			}
			level, err := validateLevel(args[0])
			if err != nil {
				return phase, nil, err
			}

			phase |= req1 | req2
			// TODO: validate provider
			// currently it is not possible, because rule validation happens on UnmarshalYAMLValidate
			// and we cannot call config.ActiveConfig.Load() because it will cause import cycle

			// err = validateNotifProvider(args[1])
			// if err != nil {
			// 	return nil, err
			// }
			return phase, &onNotifyArgs{level, args[1], titleTmpl, bodyTmpl}, nil
		},
		build: func(args any) HandlerFunc {
			level, provider, titleTmpl, bodyTmpl := args.(*onNotifyArgs).Unpack()
			to := []string{provider}

			return func(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
				var respBuf strings.Builder

				_, err := titleTmpl.ExpandVars(w, r, &respBuf)
				if err != nil {
					return err
				}
				titleLen := respBuf.Len()
				_, err = bodyTmpl.ExpandVars(w, r, &respBuf)
				if err != nil {
					return err
				}

				s := respBuf.String()
				notif.Notify(&notif.LogMessage{
					Level: level,
					Title: s[:titleLen],
					Body:  notif.MessageBodyBytes(s[titleLen:]),
					To:    to,
				})
				return nil
			}
		},
	},
}

type (
	onLogArgs    = Tuple3[zerolog.Level, io.WriteCloser, templateString]
	onNotifyArgs = Tuple4[zerolog.Level, string, templateString, templateString]
)

// Parse implements strutils.Parser.
func (cmd *Command) Parse(v string) error {
	executors, parseErr := parseDoWithBlocks(v)
	if parseErr != nil {
		return parseErr
	}

	if len(executors) == 0 {
		cmd.raw = v
		cmd.pre = nil
		cmd.post = nil
		return nil
	}

	cmd.raw = v
	for _, executor := range executors {
		if executor.Phase().IsPostRule() {
			cmd.post = append(cmd.post, executor)
		} else {
			cmd.pre = append(cmd.pre, executor)
		}
	}
	return nil
}

func (cmd *Command) String() string {
	return cmd.raw
}

func (cmd *Command) MarshalText() ([]byte, error) {
	return []byte(cmd.String()), nil
}
