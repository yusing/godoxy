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
	"github.com/yusing/godoxy/internal/auth"
	"github.com/yusing/godoxy/internal/logging"
	gphttp "github.com/yusing/godoxy/internal/net/gphttp"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/notif"
	gperr "github.com/yusing/goutils/errs"
	httputils "github.com/yusing/goutils/http"
	"github.com/yusing/goutils/http/reverseproxy"
	"github.com/yusing/goutils/synk"
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
				if !auth.AuthOrProceed(w, r) {
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
			return &Tuple[int, string]{code, text}, nil
		},
		build: func(args any) CommandHandler {
			code, text := args.(*Tuple[int, string]).Unpack()
			return TerminatingCommand(func(w http.ResponseWriter, r *http.Request) error {
				// error command should overwrite the response body
				GetInitResponseModifier(w).ResetBody()
				http.Error(w, text, code)
				return nil
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
					url.Host = r.Host
					rp := reverseproxy.NewReverseProxy(target.Host, &url, gphttp.NewTransport())
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
				helpExample(CommandLog, "info", "/dev/stdout", "{{ .Request.Method }} {{ .Request.URL }} {{ .Response.StatusCode }}"),
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
			// but will be opened only once for the same path
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
				logger = logging.NewLoggerWithLevel(level, f)
			} else {
				logger = f
			}
			return OnResponseCommand(func(w http.ResponseWriter, r *http.Request) error {
				err := executeReqRespTemplateTo(tmpl, logger, w, r)
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
				helpExample(CommandNotify, "info", "ntfy", "Received request to {{ .Request.URL }}", "{{ .Request.Method }} {{ .Response.StatusCode }}"),
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
				buf := bufPool.Get()
				defer bufPool.Put(buf)

				respBuf := bytes.NewBuffer(buf)

				err := executeReqRespTemplateTo(titleTmpl, respBuf, w, r)
				if err != nil {
					return err
				}
				titleLen := respBuf.Len()
				err = executeReqRespTemplateTo(bodyTmpl, respBuf, w, r)
				if err != nil {
					return err
				}

				notif.Notify(&notif.LogMessage{
					Level: level,
					Title: string(buf[:titleLen]),
					Body:  notif.MessageBodyBytes(buf[titleLen:]),
					To:    to,
				})
				return nil
			})
		},
	},
}

type reqResponseTemplateData struct {
	Request  *http.Request
	Response struct {
		StatusCode int
		Header     http.Header
	}
}

var bufPool = synk.GetBytesPoolWithUniqueMemory()

type onLogArgs = Tuple3[zerolog.Level, io.WriteCloser, templateOrStr]
type onNotifyArgs = Tuple4[zerolog.Level, string, templateOrStr, templateOrStr]

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
				return ErrInvalidArguments.Subject(directive)
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
	for i, exec := range executors {
		switch exec.(type) {
		case TerminatingCommand, BypassCommand:
			if i != len(executors)-1 {
				return nil, ErrInvalidCommandSequence.
					Withf("a returning / bypass command must be the last command")
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
