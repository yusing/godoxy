package rules

import (
	"net/http"
	"net/url"
	"strconv"

	httputils "github.com/yusing/goutils/http"
)

var (
	VarHeader         = "header"
	VarResponseHeader = "resp_header"
	VarCookie         = "cookie"
	VarQuery          = "arg"
	VarForm           = "form"
	VarPostForm       = "postform"
)

type dynamicVarGetter struct {
	phase PhaseFlag
	get   func(args []string, w *httputils.ResponseModifier, req *http.Request) (string, error)
}

var dynamicVarSubsMap = map[string]dynamicVarGetter{
	VarHeader: {
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
}

func getValueByKeyAtIndex[Values http.Header | url.Values](values Values, key string, index int) (string, error) {
	// NOTE: do not use Header.Get or http.CanonicalHeaderKey here, respect to user input
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
