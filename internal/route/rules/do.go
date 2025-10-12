package rules

import (
	"bytes"
	"html/template"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
	"github.com/yusing/godoxy/internal/logging"
	"github.com/yusing/godoxy/internal/logging/accesslog"
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
	CommandRewrite: {
		help: Help{
			command: CommandRewrite,
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
			return NonTerminatingCommand(func(w http.ResponseWriter, r *http.Request) {
				path := r.URL.Path
				if len(path) > 0 && path[0] != '/' {
					path = "/" + path
				}
				if !strings.HasPrefix(path, orig) {
					return
				}
				path = repl + path[len(orig):]
				r.URL.Path = path
				r.URL.RawPath = r.URL.EscapedPath()
				r.RequestURI = r.URL.RequestURI()
			})
		},
	},
	CommandServe: {
		help: Help{
			command: CommandServe,
			args: map[string]string{
				"root": "the file system path to serve, must be an existing directory",
			},
		},
		validate: validateFSPath,
		build: func(args any) CommandHandler {
			root := args.(string)
			return TerminatingCommand(func(w http.ResponseWriter, r *http.Request) {
				http.ServeFile(w, r, path.Join(root, path.Clean(r.URL.Path)))
			})
		},
	},
	CommandRedirect: {
		help: Help{
			command: CommandRedirect,
			args: map[string]string{
				"to": "the url to redirect to, can be relative or absolute URL",
			},
		},
		validate: validateURL,
		build: func(args any) CommandHandler {
			target := args.(*nettypes.URL).String()
			return TerminatingCommand(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target, http.StatusTemporaryRedirect)
			})
		},
	},
	CommandError: {
		help: Help{
			command: CommandError,
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
			return TerminatingCommand(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, text, code)
			})
		},
	},
	CommandRequireBasicAuth: {
		help: Help{
			command: CommandRequireBasicAuth,
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
			return TerminatingCommand(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
			})
		},
	},
	CommandProxy: {
		help: Help{
			command: CommandProxy,
			args: map[string]string{
				"to": "the url to proxy to, must be an absolute URL",
			},
		},
		validate: validateAbsoluteURL,
		build: func(args any) CommandHandler {
			target := args.(*nettypes.URL)
			if target.Scheme == "" {
				target.Scheme = "http"
			}
			rp := reverseproxy.NewReverseProxy("", &target.URL, gphttp.NewTransport())
			return TerminatingCommand(rp.ServeHTTP)
		},
	},
	CommandSet: {
		help: Help{
			command: CommandSet,
			args: map[string]string{
				"target": "the target to set, can be header, query, cookie",
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
			args: map[string]string{
				"target": "the target to add, can be header, query, cookie",
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
			args: map[string]string{
				"target": "the target to remove, can be header, query, cookie",
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
		help: Help{
			command: CommandLog,
			description: `The template supports the following variables:
				Request: the request object
				Response: the response object

				Example:
					log info /dev/stdout "{{ .Request.Method }} {{ .Request.URL }} {{ .Response.StatusCode }}"
				`,
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
			tmpl, err := validateTemplate(args[2])
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
			return &Tuple3[zerolog.Level, io.WriteCloser, *template.Template]{level, f, tmpl}, nil
		},
		build: func(args any) CommandHandler {
			level, f, tmpl := args.(*Tuple3[zerolog.Level, io.WriteCloser, *template.Template]).Unpack()
			var logger io.Writer
			if f == stdout || f == stderr {
				logger = logging.NewLoggerWithLevel(level, f)
			} else {
				logger = f
			}
			return OnResponseCommand(func(w http.ResponseWriter, r *http.Request) {
				var resp *http.Response
				if interceptor, ok := w.(interface {
					StatusCode() int
					Header() http.Header
				}); ok {
					resp = &http.Response{
						StatusCode: interceptor.StatusCode(),
						Header:     interceptor.Header(),
						Request:    r,
					}
				} else {
					resp = &emptyResponse
				}

				tmpl.Execute(logger, map[string]any{
					"Request":  r,
					"Response": resp,
				})
			})
		},
		isResponseHandler: true,
	},
	CommandNotify: {
		help: Help{
			command: CommandNotify,
			description: `The template supports the following variables:
				Request: the request object
				Response: the response object

				Example:
					notify info ntfy "Received request to {{ .Request.URL }}" "{{ .Request.Method }} {{ .Response.StatusCode }}"
				`,
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
			titleTmpl, err := validateTemplate(args[2])
			if err != nil {
				return nil, err
			}
			bodyTmpl, err := validateTemplate(args[3])
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

			return OnResponseCommand(func(w http.ResponseWriter, r *http.Request) {
				var resp *http.Response
				if interceptor, ok := w.(interface {
					StatusCode() int
					Header() http.Header
				}); ok {
					resp = &http.Response{
						StatusCode: interceptor.StatusCode(),
						Header:     interceptor.Header(),
						Request:    r,
					}
				} else {
					resp = &emptyResponse
				}

				buf := bufPool.Get()
				defer bufPool.Put(buf)

				respBuf := bytes.NewBuffer(buf)

				tmplData := reqResponseTemplateData{r, resp}
				titleTmpl.Execute(respBuf, tmplData)
				titleLen := respBuf.Len()
				bodyTmpl.Execute(respBuf, tmplData)

				notif.Notify(&notif.LogMessage{
					Level: level,
					Title: string(buf[:titleLen]),
					Body:  notif.MessageBodyBytes(buf[titleLen:]),
					To:    to,
				})
			})
		},
		isResponseHandler: true,
	},
}

type reqResponseTemplateData struct {
	Request  *http.Request
	Response *http.Response
}

var bufPool = synk.GetBytesPoolWithUniqueMemory()

type onNotifyArgs = Tuple4[zerolog.Level, string, *template.Template, *template.Template]

var emptyResponse http.Response

type nopCloser struct {
	io.Writer
}

func (n nopCloser) Close() error {
	return nil
}

var (
	stdout io.WriteCloser = nopCloser{os.Stdout}
	stderr io.WriteCloser = nopCloser{os.Stderr}
)

func openFile(path string) (io.WriteCloser, gperr.Error) {
	switch path {
	case "/dev/stdout":
		return stdout, nil
	case "/dev/stderr":
		return stderr, nil
	}
	f, err := accesslog.NewFileIO(path)
	if err != nil {
		return nil, ErrInvalidArguments.With(err)
	}
	return f, nil
}

// Parse implements strutils.Parser.
func (cmd *Command) Parse(v string) error {
	executors := make([]CommandHandler, 0)
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
			return err.Subject(directive).Withf("%s", builder.help.String())
		}

		executors = append(executors, builder.build(validArgs))
	}

	if len(executors) == 0 {
		return nil
	}

	exec, err := buildCmd(executors)
	if err != nil {
		return err
	}

	cmd.raw = v
	cmd.exec = exec
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

func (cmd *Command) ServeHTTP(w http.ResponseWriter, r *http.Request) (proceed bool) {
	return cmd.exec.Handle(Cache{}, w, r)
}

func (cmd *Command) String() string {
	return cmd.raw
}

func (cmd *Command) MarshalText() ([]byte, error) {
	return []byte(cmd.String()), nil
}
