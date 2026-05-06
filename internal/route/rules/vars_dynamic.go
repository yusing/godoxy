package rules

import (
	"net/http"
	"net/url"
	"strconv"

	httputils "github.com/yusing/goutils/http"
	strutils "github.com/yusing/goutils/strings"
)

const (
	VarHeader         = "header"
	VarResponseHeader = "resp_header"
	VarCookie         = "cookie"
	VarQuery          = "arg"
	VarForm           = "form"
	VarPostForm       = "postform"
	VarRedacted       = "redacted"
)

type dynamicVarGetter struct {
	help  Help
	phase PhaseFlag
	get   func(args []string, w *httputils.ResponseModifier, req *http.Request) (string, error)
}

var dynamicVarSubsMap = map[string]dynamicVarGetter{
	VarHeader: {
		help: Help{
			command: "$" + VarHeader,
			description: makeLines(
				"Request header value lookup.",
				"Uses the exact header key provided; header names are not canonicalized during lookup.",
				"$"+VarHeader+"(User-Agent)",
				"$"+VarHeader+"(X-Forwarded-For, 1)",
			),
			args: helpArgs(
				helpArg{"name", "Exact request header name."},
				helpArg{"[index]", "Optional zero-based value index; defaults to 0."},
			),
		},
		phase: PhaseNone,
		get: func(args []string, w *httputils.ResponseModifier, req *http.Request) (string, error) {
			key, index, err := getKeyAndIndex(args)
			if err != nil {
				return "", err
			}
			return getValueByKeyAtIndex(req.Header, key, index)
		},
	},
	VarResponseHeader: {
		help: Help{
			command: "$" + VarResponseHeader,
			description: makeLines(
				"Response header value lookup.",
				"Reads the current response header map in post phase using the exact key provided.",
				"$"+VarResponseHeader+"(Content-Type)",
				"$"+VarResponseHeader+"(Set-Cookie, 0)",
			),
			args: helpArgs(
				helpArg{"name", "Exact response header name."},
				helpArg{"[index]", "Optional zero-based value index; defaults to 0."},
			),
		},
		phase: PhasePost,
		get: func(args []string, w *httputils.ResponseModifier, req *http.Request) (string, error) {
			key, index, err := getKeyAndIndex(args)
			if err != nil {
				return "", err
			}
			return getValueByKeyAtIndex(w.Header(), key, index)
		},
	},
	VarCookie: {
		help: Help{
			command: "$" + VarCookie,
			description: makeLines(
				"Request cookie value lookup.",
				"Reads parsed cookies from the current forwarded request state.",
				"$"+VarCookie+"(session_id)",
				"$"+VarCookie+"(preferences, 0)",
			),
			args: helpArgs(
				helpArg{"name", "Cookie name."},
				helpArg{"[index]", "Optional zero-based value index; defaults to 0."},
			),
		},
		phase: PhaseNone,
		get: func(args []string, w *httputils.ResponseModifier, req *http.Request) (string, error) {
			key, index, err := getKeyAndIndex(args)
			if err != nil {
				return "", err
			}
			sharedData := httputils.GetSharedData(w)
			return getValueByKeyAtIndex(sharedData.GetCookiesMap(req), key, index)
		},
	},
	VarQuery: {
		help: Help{
			command: "$" + VarQuery,
			description: makeLines(
				"Query parameter value lookup.",
				"Reads the current request query parameters, including earlier rule mutations.",
				"$"+VarQuery+"(page)",
				"$"+VarQuery+"(filter, 1)",
			),
			args: helpArgs(
				helpArg{"name", "Query parameter name."},
				helpArg{"[index]", "Optional zero-based value index; defaults to 0."},
			),
		},
		phase: PhaseNone,
		get: func(args []string, w *httputils.ResponseModifier, req *http.Request) (string, error) {
			key, index, err := getKeyAndIndex(args)
			if err != nil {
				return "", err
			}
			return getValueByKeyAtIndex(httputils.GetSharedData(w).GetQueries(req), key, index)
		},
	},
	VarForm: {
		help: Help{
			command: "$" + VarForm,
			description: makeLines(
				"Parsed form value lookup with query fallback.",
				"Reads req.Form after ParseForm, so body form values and query parameters are both visible here.",
				"$"+VarForm+"(username)",
				"$"+VarForm+"(tags, 1)",
			),
			args: helpArgs(
				helpArg{"name", "Form field name."},
				helpArg{"[index]", "Optional zero-based value index; defaults to 0."},
			),
		},
		phase: PhaseNone,
		get: func(args []string, w *httputils.ResponseModifier, req *http.Request) (string, error) {
			key, index, err := getKeyAndIndex(args)
			if err != nil {
				return "", err
			}
			if req.Form == nil {
				if err := req.ParseForm(); err != nil {
					return "", err
				}
			}
			return getValueByKeyAtIndex(req.Form, key, index)
		},
	},
	VarPostForm: {
		help: Help{
			command: "$" + VarPostForm,
			description: makeLines(
				"Parsed request-body form value lookup.",
				"Reads req.PostForm from body form data only; query parameters are excluded.",
				"$"+VarPostForm+"(action)",
				"$"+VarPostForm+"(email, 0)",
			),
			args: helpArgs(
				helpArg{"name", "Request-body form field name."},
				helpArg{"[index]", "Optional zero-based value index; defaults to 0."},
			),
		},
		phase: PhaseNone,
		get: func(args []string, w *httputils.ResponseModifier, req *http.Request) (string, error) {
			key, index, err := getKeyAndIndex(args)
			if err != nil {
				return "", err
			}
			if req.Form == nil {
				if err := req.ParseForm(); err != nil {
					return "", err
				}
			}
			return getValueByKeyAtIndex(req.PostForm, key, index)
		},
	},
	// VarRedacted wraps the result of its single argument (which may be another dynamic var
	// expression, already expanded by expandArgs) with strutils.Redact.
	VarRedacted: {
		help: Help{
			command: "$" + VarRedacted,
			description: makeLines(
				"Mask a sensitive value for logs or notifications.",
				"Keeps only the edges of the input visible and replaces the middle with asterisks.",
				"$"+VarRedacted+"($header(Authorization))",
				"$"+VarRedacted+"($cookie(session_id))",
			),
			args: helpArgs(
				helpArg{"value", "Literal text or the output of another variable expression."},
			),
		},
		phase: PhaseNone,
		get: func(args []string, w *httputils.ResponseModifier, req *http.Request) (string, error) {
			if len(args) != 1 {
				return "", ErrExpectOneArg
			}
			return strutils.Redact(args[0]), nil
		},
	},
}

func getValueByKeyAtIndex[Values http.Header | url.Values](values Values, key string, index int) (string, error) {
	// NOTE: do not use Header.Get or http.CanonicalHeaderKey here, respect to user's input
	if values, ok := values[key]; ok && index < len(values) {
		return stripFragment(values[index]), nil
	}
	// ignore unknown header or index out of range
	return "", nil
}

func getKeyAndIndex(args []string) (key string, index int, err error) {
	switch len(args) {
	case 0:
		return "", 0, ErrExpectNoArg
	case 1:
		return args[0], 0, nil
	case 2:
		index, err = strconv.Atoi(args[1])
		if err != nil {
			return "", 0, ErrInvalidArguments.Withf("invalid index %q", args[1])
		}
		return args[0], index, nil
	default:
		return "", 0, ErrExpectOneOrTwoArgs
	}
}
