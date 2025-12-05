package rules

import (
	"io"
	"net/http"
	"net/url"
	"strconv"

	gperr "github.com/yusing/goutils/errs"
	httputils "github.com/yusing/goutils/http"
	ioutils "github.com/yusing/goutils/io"
)

type (
	FieldHandler struct {
		set, add, remove CommandHandler
	}
	FieldModifier string
)

const (
	ModFieldSet    FieldModifier = "set"
	ModFieldAdd    FieldModifier = "add"
	ModFieldRemove FieldModifier = "remove"
)

const (
	FieldHeader         = "header"
	FieldResponseHeader = "resp_header"
	FieldQuery          = "query"
	FieldCookie         = "cookie"
	FieldBody           = "body"
	FieldResponseBody   = "resp_body"
	FieldStatusCode     = "status"
)

var AllFields = []string{FieldHeader, FieldResponseHeader, FieldQuery, FieldCookie, FieldBody, FieldResponseBody, FieldStatusCode}

// NOTE: should not use canonicalized header keys, respect to user's input
var modFields = map[string]struct {
	help     Help
	validate ValidateFunc
	builder  func(args any) *FieldHandler
}{
	FieldHeader: {
		help: Help{
			command: FieldHeader,
			args: map[string]string{
				"key":   "the header key",
				"value": "the header template",
			},
		},
		validate: toKeyValueTemplate,
		builder: func(args any) *FieldHandler {
			k, tmpl := args.(*keyValueTemplate).Unpack()
			return &FieldHandler{
				set: NonTerminatingCommand(func(w http.ResponseWriter, r *http.Request) error {
					v, err := tmpl.ExpandVarsToString(w, r)
					if err != nil {
						return err
					}
					r.Header[k] = []string{v}
					return nil
				}),
				add: NonTerminatingCommand(func(w http.ResponseWriter, r *http.Request) error {
					v, err := tmpl.ExpandVarsToString(w, r)
					if err != nil {
						return err
					}
					r.Header[k] = append(r.Header[k], v)
					return nil
				}),
				remove: NonTerminatingCommand(func(w http.ResponseWriter, r *http.Request) error {
					delete(r.Header, k)
					return nil
				}),
			}
		},
	},
	FieldResponseHeader: {
		help: Help{
			command: FieldResponseHeader,
			args: map[string]string{
				"key":   "the response header key",
				"value": "the response header template",
			},
		},
		validate: toKeyValueTemplate,
		builder: func(args any) *FieldHandler {
			k, tmpl := args.(*keyValueTemplate).Unpack()
			return &FieldHandler{
				set: NonTerminatingCommand(func(w http.ResponseWriter, r *http.Request) error {
					v, err := tmpl.ExpandVarsToString(w, r)
					if err != nil {
						return err
					}
					w.Header()[k] = []string{v}
					return nil
				}),
				add: NonTerminatingCommand(func(w http.ResponseWriter, r *http.Request) error {
					v, err := tmpl.ExpandVarsToString(w, r)
					if err != nil {
						return err
					}
					w.Header()[k] = append(w.Header()[k], v)
					return nil
				}),
				remove: NonTerminatingCommand(func(w http.ResponseWriter, r *http.Request) error {
					delete(w.Header(), k)
					return nil
				}),
			}
		},
	},
	FieldQuery: {
		help: Help{
			command: FieldQuery,
			args: map[string]string{
				"key":   "the query key",
				"value": "the query template",
			},
		},
		validate: toKeyValueTemplate,
		builder: func(args any) *FieldHandler {
			k, tmpl := args.(*keyValueTemplate).Unpack()
			return &FieldHandler{
				set: NonTerminatingCommand(func(w http.ResponseWriter, r *http.Request) error {
					v, err := tmpl.ExpandVarsToString(w, r)
					if err != nil {
						return err
					}
					httputils.GetSharedData(w).UpdateQueries(r, func(queries url.Values) {
						queries.Set(k, v)
					})
					return nil
				}),
				add: NonTerminatingCommand(func(w http.ResponseWriter, r *http.Request) error {
					v, err := tmpl.ExpandVarsToString(w, r)
					if err != nil {
						return err
					}
					httputils.GetSharedData(w).UpdateQueries(r, func(queries url.Values) {
						queries.Add(k, v)
					})
					return nil
				}),
				remove: NonTerminatingCommand(func(w http.ResponseWriter, r *http.Request) error {
					httputils.GetSharedData(w).UpdateQueries(r, func(queries url.Values) {
						queries.Del(k)
					})
					return nil
				}),
			}
		},
	},
	FieldCookie: {
		help: Help{
			command: FieldCookie,
			args: map[string]string{
				"key":   "the cookie key",
				"value": "the cookie value",
			},
		},
		validate: toKeyValueTemplate,
		builder: func(args any) *FieldHandler {
			k, tmpl := args.(*keyValueTemplate).Unpack()
			return &FieldHandler{
				set: NonTerminatingCommand(func(w http.ResponseWriter, r *http.Request) error {
					v, err := tmpl.ExpandVarsToString(w, r)
					if err != nil {
						return err
					}
					httputils.GetSharedData(w).UpdateCookies(r, func(cookies []*http.Cookie) []*http.Cookie {
						for i, c := range cookies {
							if c.Name == k {
								cookies[i].Value = v
								return cookies
							}
						}
						return append(cookies, &http.Cookie{Name: k, Value: v})
					})
					return nil
				}),
				add: NonTerminatingCommand(func(w http.ResponseWriter, r *http.Request) error {
					v, err := tmpl.ExpandVarsToString(w, r)
					if err != nil {
						return err
					}
					httputils.GetSharedData(w).UpdateCookies(r, func(cookies []*http.Cookie) []*http.Cookie {
						return append(cookies, &http.Cookie{Name: k, Value: v})
					})
					return nil
				}),
				remove: NonTerminatingCommand(func(w http.ResponseWriter, r *http.Request) error {
					httputils.GetSharedData(w).UpdateCookies(r, func(cookies []*http.Cookie) []*http.Cookie {
						index := -1
						for i, c := range cookies {
							if c.Name == k {
								index = i
								break
							}
						}
						if index != -1 {
							if len(cookies) == 1 {
								return []*http.Cookie{}
							}
							return append(cookies[:index], cookies[index+1:]...)
						}
						return cookies
					})
					return nil
				}),
			}
		},
	},
	FieldBody: {
		help: Help{
			command: FieldBody,
			description: makeLines(
				"Override the request body that will be sent to the upstream",
				"The template supports the following variables:",
				helpListItem("Request", "the request object"),
				"",
				"Example:",
				helpExample(FieldBody, "HTTP STATUS: $req_method $req_path"),
			),
			args: map[string]string{
				"template": "the body template",
			},
		},
		validate: func(args []string) (any, gperr.Error) {
			if len(args) != 1 {
				return nil, ErrExpectOneArg
			}
			return validateTemplate(args[0], true)
		},
		builder: func(args any) *FieldHandler {
			tmpl := args.(templateString)
			return &FieldHandler{
				set: NonTerminatingCommand(func(w http.ResponseWriter, r *http.Request) error {
					if r.Body != nil {
						r.Body.Close()
						r.Body = nil
					}

					bufPool := httputils.GetInitResponseModifier(w).BufPool()
					b := bufPool.GetBuffer()
					err := tmpl.ExpandVars(w, r, b)
					if err != nil {
						return err
					}
					r.Body = ioutils.NewHookReadCloser(io.NopCloser(b), func() {
						bufPool.PutBuffer(b)
					})
					return nil
				}),
			}
		},
	},
	FieldResponseBody: {
		help: Help{
			command: FieldResponseBody,
			description: makeLines(
				"Override the response body that will be sent to the client",
				"The template supports the following variables:",
				helpListItem("Request", "the request object"),
				helpListItem("Response", "the response object"),
				"",
				"Example:",
				helpExample(FieldResponseBody, "HTTP STATUS: $req_method $status_code"),
			),
			args: map[string]string{
				"template": "the response body template",
			},
		},
		validate: func(args []string) (any, gperr.Error) {
			if len(args) != 1 {
				return nil, ErrExpectOneArg
			}
			return validateTemplate(args[0], true)
		},
		builder: func(args any) *FieldHandler {
			tmpl := args.(templateString)
			return &FieldHandler{
				set: OnResponseCommand(func(w http.ResponseWriter, r *http.Request) error {
					rm := httputils.GetInitResponseModifier(w)
					rm.ResetBody()
					return tmpl.ExpandVars(w, r, rm)
				}),
			}
		},
	},
	FieldStatusCode: {
		help: Help{
			command: FieldStatusCode,
			description: makeLines(
				"Override the status code that will be sent to the client, e.g.:",
				helpExample(FieldStatusCode, "200"),
			),
			args: map[string]string{
				"code": "the status code",
			},
		},
		validate: func(args []string) (any, gperr.Error) {
			if len(args) != 1 {
				return nil, ErrExpectOneArg
			}
			status, err := strconv.Atoi(args[0])
			if err != nil {
				return nil, ErrInvalidArguments.With(err)
			}
			if status < 100 || status > 599 {
				return nil, ErrInvalidArguments.Withf("status code must be between 100 and 599, got %d", status)
			}
			return status, nil
		},
		builder: func(args any) *FieldHandler {
			status := args.(int)
			return &FieldHandler{
				set: NonTerminatingCommand(func(w http.ResponseWriter, r *http.Request) error {
					httputils.GetInitResponseModifier(w).WriteHeader(status)
					return nil
				}),
			}
		},
	},
}
