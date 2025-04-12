package servemux

import (
	"fmt"
	"net/http"

	"github.com/yusing/go-proxy/internal/api/v1/auth"
	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/net/gphttp/httpheaders"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

type (
	ServeMux struct {
		*http.ServeMux
		cfg config.ConfigInstance
	}
	WithCfgHandler = func(config.ConfigInstance, http.ResponseWriter, *http.Request)
)

func NewServeMux(cfg config.ConfigInstance) ServeMux {
	return ServeMux{http.NewServeMux(), cfg}
}

func (mux ServeMux) HandleFunc(methods, endpoint string, h any, requireAuth ...bool) {
	var handler http.HandlerFunc
	switch h := h.(type) {
	case func(http.ResponseWriter, *http.Request):
		handler = h
	case http.Handler:
		handler = h.ServeHTTP
	case WithCfgHandler:
		handler = func(w http.ResponseWriter, r *http.Request) {
			h(mux.cfg, w, r)
		}
	default:
		panic(fmt.Errorf("unsupported handler type: %T", h))
	}

	matchDomains := mux.cfg.Value().MatchDomains
	if len(matchDomains) > 0 {
		origHandler := handler
		handler = func(w http.ResponseWriter, r *http.Request) {
			if httpheaders.IsWebsocket(r.Header) {
				httpheaders.SetWebsocketAllowedDomains(r.Header, matchDomains)
			}
			origHandler(w, r)
		}
	}

	if len(requireAuth) > 0 && requireAuth[0] {
		handler = auth.RequireAuth(handler)
	}
	if methods == "" {
		mux.ServeMux.HandleFunc(endpoint, handler)
	} else {
		for _, m := range strutils.CommaSeperatedList(methods) {
			mux.ServeMux.HandleFunc(m+" "+endpoint, handler)
		}
	}
}
