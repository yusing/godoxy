package rules

import (
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/yusing/go-proxy/internal/gperr"
	gphttp "github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/net/gphttp/reverseproxy"
	nettypes "github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

type (
	Command struct {
		raw  string
		exec CommandHandler
	}
)

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
	CommandPass             = "pass"
	CommandPassAlt          = "bypass"
)

var commands = map[string]struct {
	help     Help
	validate ValidateFunc
	build    func(args any) CommandHandler
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
			return validateURLPaths(args)
		},
		build: func(args any) CommandHandler {
			a := args.([]string)
			orig, repl := a[0], a[1]
			return StaticCommand(func(w http.ResponseWriter, r *http.Request) {
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
			return ReturningCommand(func(w http.ResponseWriter, r *http.Request) {
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
			return ReturningCommand(func(w http.ResponseWriter, r *http.Request) {
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
			if !gphttp.IsStatusCodeValid(code) {
				return nil, ErrInvalidArguments.Subject(codeStr)
			}
			return &Tuple[int, string]{code, text}, nil
		},
		build: func(args any) CommandHandler {
			code, text := args.(*Tuple[int, string]).Unpack()
			return ReturningCommand(func(w http.ResponseWriter, r *http.Request) {
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
			return ReturningCommand(func(w http.ResponseWriter, r *http.Request) {
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
			rp := reverseproxy.NewReverseProxy("", target, gphttp.NewTransport())
			return ReturningCommand(rp.ServeHTTP)
		},
	},
	CommandSet: {
		help: Help{
			command: CommandSet,
			args: map[string]string{
				"field": "the field to set",
				"value": "the value to set",
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
				"field": "the field to add",
				"value": "the value to add",
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
				"field": "the field to remove",
			},
		},
		validate: func(args []string) (any, gperr.Error) {
			return validateModField(ModFieldRemove, args)
		},
		build: func(args any) CommandHandler {
			return args.(CommandHandler)
		},
	},
}

// Parse implements strutils.Parser.
func (cmd *Command) Parse(v string) error {
	lines := strutils.SplitLine(v)
	if len(lines) == 0 {
		return nil
	}

	executors := make([]CommandHandler, 0, len(lines))
	for _, line := range lines {
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

func buildCmd(executors []CommandHandler) (CommandHandler, error) {
	for i, exec := range executors {
		switch exec.(type) {
		case ReturningCommand, BypassCommand:
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

func (cmd *Command) String() string {
	return cmd.raw
}

func (cmd *Command) MarshalText() ([]byte, error) {
	return []byte(cmd.String()), nil
}
