package rules

import (
	"bytes"
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
		raw               string
		exec              CommandHandler
		isResponseHandler bool
	}
)

func (cmd *Command) IsResponseHandler() bool {
	return cmd.isResponseHandler
}

const (
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
	CommandPass             = "pass"
	CommandPassAlt          = "bypass"
)

type AuthHandler func(w http.ResponseWriter, r *http.Request) (proceed bool)

var authHandler AuthHandler

func InitAuthHandler(handler AuthHandler) {
	authHandler = handler
}

var commands = map[string]struct {
	help              Help
	validate          ValidateFunc
	build             func(args any) CommandHandler
	isResponseHandler bool
}{
	CommandRequireAuth: {
		help: Help{
			command:     CommandRequireAuth,
			description: makeLines("Require HTTP authentication for incoming requests"),
			args:        map[string]string{},
		},
		validate: func(args []string) (any, gperr.Error) {
			if len(args) != 0 {
				return nil, ErrExpectNoArg
			}
			return nil, nil
		},
		build: func(args any) CommandHandler {
			return NonTerminatingCommand(func(w http.ResponseWriter, r *http.Request) error {
				if authHandler == nil {
					http.Error(w, "Auth handler not initialized", http.StatusInternalServerError)
					return errTerminated
				}
				if !authHandler(w, r) {
					return errTerminated
				}
				return nil
			})
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
		validate: func(args []string) (any, gperr.Error) {
			if len(args) != 2 {
				return nil, ErrExpectTwoArgs
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
				return nil, gperr.Join(err1, err2)
			}
			return &StrTuple{path1.(string), path2.(string)}, nil
		},
		build: func(args any) CommandHandler {
			orig, repl := args.(*StrTuple).Unpack()
			return NonTerminatingCommand(func(w http.ResponseWriter, r *http.Request) error {
				path := r.URL.Path
				if len(path) > 0 && path[0] != '/' {
					path = "/" + path
				}
				if !strings.HasPrefix(path, orig) {
					return nil
				}
				path = repl + path[len(orig):]
				r.URL.Path = path
				r.URL.RawPath = r.URL.EscapedPath()
				r.RequestURI = r.URL.RequestURI()
				return nil
			})
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
		validate: validateFSPath,
		build: func(args any) CommandHandler {
			root := args.(string)
			return TerminatingCommand(func(w http.ResponseWriter, r *http.Request) error {
				http.ServeFile(w, r, path.Join(root, path.Clean(r.URL.Path)))
				return nil
			})
		},
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
		validate: validateURL,
		build: func(args any) CommandHandler {
			target := args.(*nettypes.URL).String()
			return TerminatingCommand(func(w http.ResponseWriter, r *http.Request) error {
				http.Redirect(w, r, target, http.StatusTemporaryRedirect)
				return nil
			})
		},
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
		validate: func(args []string) (any, gperr.Error) {
			if len(args) != 1 {
				return nil, ErrExpectOneArg
			}
			return args[0], nil
		},
		build: func(args any) CommandHandler {
			route := args.(string)
			return TerminatingCommand(func(w http.ResponseWriter, req *http.Request) error {
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
				return nil
			})
		},
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
		validate: func(args []string) (any, gperr.Error) {
			if len(args) != 2 {
				return nil, ErrExpectTwoArgs
			}
			codeStr, text := args[0], args[1]
			code, err := strconv.Atoi(codeStr)
			if err != nil {
				return nil, ErrInvalidArguments.With(err)
			}
			if !httputils.IsStatusCodeValid(code) {
				return nil, ErrInvalidArguments.Subject(codeStr)
			}
			textTmpl, err := validateTemplate(text, true)
			if err != nil {
				return nil, ErrInvalidArguments.With(err)
			}
			return &Tuple[int, templateString]{code, textTmpl}, nil
		},
		build: func(args any) CommandHandler {
			code, textTmpl := args.(*Tuple[int, templateString]).Unpack()
			return TerminatingCommand(func(w http.ResponseWriter, r *http.Request) error {
				// error command should overwrite the response body
				httputils.GetInitResponseModifier(w).ResetBody()
				w.WriteHeader(code)
				err := textTmpl.ExpandVars(w, r, w)
				return err
			})
		},
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
		validate: func(args []string) (any, gperr.Error) {
			if len(args) == 1 {
				return args[0], nil
			}
			return nil, ErrExpectOneArg
		},
		build: func(args any) CommandHandler {
			realm := args.(string)
			return TerminatingCommand(func(w http.ResponseWriter, r *http.Request) error {
				w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return nil
			})
		},
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
		validate: validateURL,
		build: func(args any) CommandHandler {
			target := args.(*nettypes.URL)
			if target.Scheme == "" {
				target.Scheme = "http"
			}
			if target.Host == "" {
				return TerminatingCommand(func(w http.ResponseWriter, r *http.Request) error {
					url := target.URL
					url.Host = routes.TryGetUpstreamHostPort(r)
					if url.Host == "" {
						return fmt.Errorf("no upstream host: %s", r.URL.String())
					}
					rp := reverseproxy.NewReverseProxy(url.Host, &url, gphttp.NewTransport())
					r.URL.Path = target.Path
					r.URL.RawPath = r.URL.EscapedPath()
					r.RequestURI = r.URL.RequestURI()
					rp.ServeHTTP(w, r)
					return nil
				})
			}
			rp := reverseproxy.NewReverseProxy("", &target.URL, gphttp.NewTransport())
			return TerminatingCommand(func(w http.ResponseWriter, r *http.Request) error {
				rp.ServeHTTP(w, r)
				return nil
			})
		},
	},
	CommandSet: {
		help: Help{
			command: CommandSet,
			description: makeLines(
				"Set a field in the request or response, e.g.:",
				helpExample(CommandSet, "header", "User-Agent", "godoxy"),
			),
			args: map[string]string{
				"target": fmt.Sprintf("the target to set, can be %s", strings.Join(AllFields, ", ")),
				"field":  "the field to set",
				"value":  "the value to set",
			},
		},
		validate: func(args []string) (any, gperr.Error) {
			return validateModField(ModFieldSet, args)
		},
		build: func(args any) CommandHandler {
			return args.(CommandHandler)
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
				"target": fmt.Sprintf("the target to add, can be %s", strings.Join(AllFields, ", ")),
				"field":  "the field to add",
				"value":  "the value to add",
			},
		},
		validate: func(args []string) (any, gperr.Error) {
			return validateModField(ModFieldAdd, args)
		},
		build: func(args any) CommandHandler {
			return args.(CommandHandler)
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
				"target": fmt.Sprintf("the target to remove, can be %s", strings.Join(AllFields, ", ")),
				"field":  "the field to remove",
			},
		},
		validate: func(args []string) (any, gperr.Error) {
			return validateModField(ModFieldRemove, args)
		},
		build: func(args any) CommandHandler {
			return args.(CommandHandler)
		},
	},
	CommandLog: {
		isResponseHandler: true,
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
		validate: func(args []string) (any, gperr.Error) {
			if len(args) != 3 {
				return nil, ErrExpectThreeArgs
			}
			tmpl, err := validateTemplate(args[2], true)
			if err != nil {
				return nil, err
			}
			level, err := validateLevel(args[0])
			if err != nil {
				return nil, err
			}
			// NOTE: file will stay opened forever
			// it leverages accesslog.NewFileIO so
			// it will be opened only once for the same path
			f, err := openFile(args[1])
			if err != nil {
				return nil, err
			}
			return &onLogArgs{level, f, tmpl}, nil
		},
		build: func(args any) CommandHandler {
			level, f, tmpl := args.(*onLogArgs).Unpack()
			var logger io.Writer
			if f == stdout || f == stderr {
				logger = logging.NewLoggerWithFixedLevel(level, f)
			} else {
				logger = f
			}
			return OnResponseCommand(func(w http.ResponseWriter, r *http.Request) error {
				err := tmpl.ExpandVars(w, r, logger)
				if err != nil {
					return err
				}
				return nil
			})
		},
	},
	CommandNotify: {
		isResponseHandler: true,
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
		validate: func(args []string) (any, gperr.Error) {
			if len(args) != 4 {
				return nil, ErrExpectFourArgs
			}
			titleTmpl, err := validateTemplate(args[2], false)
			if err != nil {
				return nil, err
			}
			bodyTmpl, err := validateTemplate(args[3], false)
			if err != nil {
				return nil, err
			}
			level, err := validateLevel(args[0])
			if err != nil {
				return nil, err
			}
			// TODO: validate provider
			// currently it is not possible, because rule validation happens on UnmarshalYAMLValidate
			// and we cannot call config.ActiveConfig.Load() because it will cause import cycle

			// err = validateNotifProvider(args[1])
			// if err != nil {
			// 	return nil, err
			// }
			return &onNotifyArgs{level, args[1], titleTmpl, bodyTmpl}, nil
		},
		build: func(args any) CommandHandler {
			level, provider, titleTmpl, bodyTmpl := args.(*onNotifyArgs).Unpack()
			to := []string{provider}

			return OnResponseCommand(func(w http.ResponseWriter, r *http.Request) error {
				respBuf := bytes.NewBuffer(make([]byte, 0, titleTmpl.Len()+bodyTmpl.Len()))

				err := titleTmpl.ExpandVars(w, r, respBuf)
				if err != nil {
					return err
				}
				titleLen := respBuf.Len()
				err = bodyTmpl.ExpandVars(w, r, respBuf)
				if err != nil {
					return err
				}

				b := respBuf.Bytes()
				notif.Notify(&notif.LogMessage{
					Level: level,
					Title: string(b[:titleLen]),
					Body:  notif.MessageBodyBytes(b[titleLen:]),
					To:    to,
				})
				return nil
			})
		},
	},
}

type onLogArgs = Tuple3[zerolog.Level, io.WriteCloser, templateString]
type onNotifyArgs = Tuple4[zerolog.Level, string, templateString, templateString]

// Parse implements strutils.Parser.
func (cmd *Command) Parse(v string) error {
	executors := make([]CommandHandler, 0)
	isResponseHandler := false
	for line := range strings.SplitSeq(v, "\n") {
		if line == "" {
			continue
		}

		directive, args, err := parse(line)
		if err != nil {
			return err
		}

		if directive == CommandPass || directive == CommandPassAlt {
			if len(args) != 0 {
				return ErrExpectNoArg
			}
			executors = append(executors, BypassCommand{})
			continue
		}

		builder, ok := commands[directive]
		if !ok {
			return ErrUnknownDirective.Subject(directive)
		}
		validArgs, err := builder.validate(args)
		if err != nil {
			// Only attach help for the directive that failed, avoid bringing in unrelated KV errors
			return err.Subject(directive).With(builder.help.Error())
		}

		handler := builder.build(validArgs)
		executors = append(executors, handler)
		if builder.isResponseHandler || handler.IsResponseHandler() {
			isResponseHandler = true
		}
	}

	if len(executors) == 0 {
		cmd.raw = v
		cmd.exec = nil
		cmd.isResponseHandler = false
		return nil
	}

	exec, err := buildCmd(executors)
	if err != nil {
		return err
	}

	cmd.raw = v
	cmd.exec = exec
	if exec.IsResponseHandler() {
		isResponseHandler = true
	}
	cmd.isResponseHandler = isResponseHandler
	return nil
}

func buildCmd(executors []CommandHandler) (cmd CommandHandler, err error) {
	// Validate the execution order.
	//
	// This allows sequences like:
	//   route ws-api
	//   log info /dev/stdout "..."
	// where the first command is request-phase and the last is response-phase.
	lastNonResp := -1
	seenResp := false
	for i, exec := range executors {
		if exec.IsResponseHandler() {
			seenResp = true
			continue
		}
		if seenResp {
			return nil, ErrInvalidCommandSequence.Withf("response handlers must be the last commands")
		}
		lastNonResp = i
	}

	for i, exec := range executors {
		if i > lastNonResp {
			break // response-handler tail
		}
		switch exec.(type) {
		case TerminatingCommand, BypassCommand:
			if i != lastNonResp {
				return nil, ErrInvalidCommandSequence.
					Withf("a response handler or terminating/bypass command must be the last command")
			}
		}
	}

	return Commands(executors), nil
}

// Command is purely "bypass" or empty.
func (cmd *Command) isBypass() bool {
	if cmd == nil {
		return true
	}
	switch cmd := cmd.exec.(type) {
	case BypassCommand:
		return true
	case Commands:
		// bypass command is always the last one
		_, ok := cmd[len(cmd)-1].(BypassCommand)
		return ok
	default:
		return false
	}
}

func (cmd *Command) ServeHTTP(w http.ResponseWriter, r *http.Request) error {
	return cmd.exec.Handle(w, r)
}

func (cmd *Command) String() string {
	return cmd.raw
}

func (cmd *Command) MarshalText() ([]byte, error) {
	return []byte(cmd.String()), nil
}
