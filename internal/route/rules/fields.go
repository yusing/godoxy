package rules

import (
	"net/http"
	"net/url"
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
	FieldHeader = "header"
	FieldQuery  = "query"
	FieldCookie = "cookie"
)

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
				"value": "the header value",
			},
		},
		validate: toStrTuple,
		builder: func(args any) *FieldHandler {
			k, v := args.(*StrTuple).Unpack()
			return &FieldHandler{
				set: NonTerminatingCommand(func(w http.ResponseWriter, r *http.Request) {
					w.Header()[k] = []string{v}
				}),
				add: NonTerminatingCommand(func(w http.ResponseWriter, r *http.Request) {
					h := w.Header()
					h[k] = append(h[k], v)
				}),
				remove: NonTerminatingCommand(func(w http.ResponseWriter, r *http.Request) {
					delete(w.Header(), k)
				}),
			}
		},
	},
	FieldQuery: {
		help: Help{
			command: FieldQuery,
			args: map[string]string{
				"key":   "the query key",
				"value": "the query value",
			},
		},
		validate: toStrTuple,
		builder: func(args any) *FieldHandler {
			k, v := args.(*StrTuple).Unpack()
			return &FieldHandler{
				set: DynamicCommand(func(cached Cache, w http.ResponseWriter, r *http.Request) bool {
					cached.UpdateQueries(r, func(queries url.Values) {
						queries.Set(k, v)
					})
					return true
				}),
				add: DynamicCommand(func(cached Cache, w http.ResponseWriter, r *http.Request) bool {
					cached.UpdateQueries(r, func(queries url.Values) {
						queries.Add(k, v)
					})
					return true
				}),
				remove: DynamicCommand(func(cached Cache, w http.ResponseWriter, r *http.Request) bool {
					cached.UpdateQueries(r, func(queries url.Values) {
						queries.Del(k)
					})
					return true
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
		validate: toStrTuple,
		builder: func(args any) *FieldHandler {
			k, v := args.(*StrTuple).Unpack()
			return &FieldHandler{
				set: DynamicCommand(func(cached Cache, w http.ResponseWriter, r *http.Request) bool {
					cached.UpdateCookies(r, func(cookies []*http.Cookie) []*http.Cookie {
						for i, c := range cookies {
							if c.Name == k {
								cookies[i].Value = v
								return cookies
							}
						}
						return append(cookies, &http.Cookie{Name: k, Value: v})
					})
					return true
				}),
				add: DynamicCommand(func(cached Cache, w http.ResponseWriter, r *http.Request) bool {
					cached.UpdateCookies(r, func(cookies []*http.Cookie) []*http.Cookie {
						return append(cookies, &http.Cookie{Name: k, Value: v})
					})
					return true
				}),
				remove: DynamicCommand(func(cached Cache, w http.ResponseWriter, r *http.Request) bool {
					cached.UpdateCookies(r, func(cookies []*http.Cookie) []*http.Cookie {
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
					return true
				}),
			}
		},
	},
}
