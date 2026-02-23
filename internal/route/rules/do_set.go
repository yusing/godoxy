package rules

import (
	"io"
	"net/http"
	"net/url"
	"strconv"

	httputils "github.com/yusing/goutils/http"
	ioutils "github.com/yusing/goutils/io"
)

type (
	FieldHandler struct {
		set, add, remove HandlerFunc
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
		validate: validatePreRequestKVTemplate,
		builder: func(args any) *FieldHandler {
			k, tmpl := args.(*keyValueTemplate).Unpack()
			return &FieldHandler{
				set: func(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
					v, _, err := tmpl.ExpandVarsToString(w, r)
					if err != nil {
						return err
					}
					r.Header[k] = []string{v}
					return nil
				},
				add: func(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
					v, _, err := tmpl.ExpandVarsToString(w, r)
					if err != nil {
						return err
					}
					r.Header[k] = append(r.Header[k], v)
					return nil
				},
				remove: func(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
					delete(r.Header, k)
					return nil
				},
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
		validate: validatePostResponseKVTemplate,
		builder: func(args any) *FieldHandler {
			k, tmpl := args.(*keyValueTemplate).Unpack()
			return &FieldHandler{
				set: func(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
					v, _, err := tmpl.ExpandVarsToString(w, r)
					if err != nil {
						return err
					}
					w.Header()[k] = []string{v}
					return nil
				},
				add: func(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
					v, _, err := tmpl.ExpandVarsToString(w, r)
					if err != nil {
						return err
					}
					w.Header()[k] = append(w.Header()[k], v)
					return nil
				},
				remove: func(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
					delete(w.Header(), k)
					return nil
				},
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
		validate: validatePreRequestKVTemplate,
		builder: func(args any) *FieldHandler {
			k, tmpl := args.(*keyValueTemplate).Unpack()
			return &FieldHandler{
				set: func(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
					v, _, err := tmpl.ExpandVarsToString(w, r)
					if err != nil {
						return err
					}
					w.SharedData().UpdateQueries(r, func(queries url.Values) {
						queries.Set(k, v)
					})
					return nil
				},
				add: func(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
					v, _, err := tmpl.ExpandVarsToString(w, r)
					if err != nil {
						return err
					}
					w.SharedData().UpdateQueries(r, func(queries url.Values) {
						queries.Add(k, v)
					})
					return nil
				},
				remove: func(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
					w.SharedData().UpdateQueries(r, func(queries url.Values) {
						queries.Del(k)
					})
					return nil
				},
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
		validate: validatePreRequestKVTemplate,
		builder: func(args any) *FieldHandler {
			k, tmpl := args.(*keyValueTemplate).Unpack()
			return &FieldHandler{
				set: func(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
					v, _, err := tmpl.ExpandVarsToString(w, r)
					if err != nil {
						return err
					}
					w.SharedData().UpdateCookies(r, func(cookies []*http.Cookie) []*http.Cookie {
						for i, c := range cookies {
							if c.Name == k {
								cookies[i].Value = v
								return cookies
							}
						}
						return append(cookies, &http.Cookie{Name: k, Value: v})
					})
					return nil
				},
				add: func(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
					v, _, err := tmpl.ExpandVarsToString(w, r)
					if err != nil {
						return err
					}
					w.SharedData().UpdateCookies(r, func(cookies []*http.Cookie) []*http.Cookie {
						return append(cookies, &http.Cookie{Name: k, Value: v})
					})
					return nil
				},
				remove: func(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
					w.SharedData().UpdateCookies(r, func(cookies []*http.Cookie) []*http.Cookie {
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
				},
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
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			if len(args) != 1 {
				return 0, nil, ErrExpectOneArg
			}
			phase = PhasePre
			tmplReq, parsedArgs, err := validateTemplate(args[0], true)
			phase |= tmplReq
			return
		},
		builder: func(args any) *FieldHandler {
			tmpl := args.(templateString)
			return &FieldHandler{
				set: func(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
					if r.Body != nil {
						r.Body.Close()
						r.Body = nil
					}

					bufPool := w.BufPool()
					b := bufPool.GetBuffer()
					_, err := tmpl.ExpandVars(w, r, b)
					if err != nil {
						return err
					}
					r.Body = ioutils.NewHookReadCloser(io.NopCloser(b), func() {
						bufPool.PutBuffer(b)
					})
					return nil
				},
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
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			if len(args) != 1 {
				return 0, nil, ErrExpectOneArg
			}
			phase = PhasePost
			tmplReq, parsedArgs, err := validateTemplate(args[0], true)
			phase |= tmplReq
			return
		},
		builder: func(args any) *FieldHandler {
			tmpl := args.(templateString)
			return &FieldHandler{
				set: func(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
					w.ResetBody()
					_, err := tmpl.ExpandVars(w, r, w)
					if err != nil {
						return err
					}
					return nil
				},
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
		validate: func(args []string) (phase PhaseFlag, parsedArgs any, err error) {
			if len(args) != 1 {
				return phase, nil, ErrExpectOneArg
			}
			phase = PhasePost
			status, err := strconv.Atoi(args[0])
			if err != nil {
				return phase, nil, ErrInvalidArguments.With(err)
			}
			if status < 100 || status > 599 {
				return phase, nil, ErrInvalidArguments.Withf("status code must be between 100 and 599, got %d", status)
			}
			return phase, status, nil
		},
		builder: func(args any) *FieldHandler {
			status := args.(int)
			return &FieldHandler{
				set: func(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
					w.WriteHeader(status)
					return nil
				},
			}
		},
	},
}
