package accesslog

import (
	"net"
	"net/http"
	"strings"

	"github.com/yusing/go-proxy/internal/gperr"
	nettypes "github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

type (
	LogFilter[T Filterable] struct {
		Negative bool
		Values   []T
	}
	Filterable interface {
		comparable
		Fulfill(req *http.Request, res *http.Response) bool
	}
	HTTPMethod string
	HTTPHeader struct {
		Key, Value string
	}
	Host string
	CIDR struct {
		nettypes.CIDR
	}
)

var ErrInvalidHTTPHeaderFilter = gperr.New("invalid http header filter")

func (f *LogFilter[T]) CheckKeep(req *http.Request, res *http.Response) bool {
	if len(f.Values) == 0 {
		return !f.Negative
	}
	for _, check := range f.Values {
		if check.Fulfill(req, res) {
			return !f.Negative
		}
	}
	return f.Negative
}

func (r *StatusCodeRange) Fulfill(req *http.Request, res *http.Response) bool {
	return r.Includes(res.StatusCode)
}

func (method HTTPMethod) Fulfill(req *http.Request, res *http.Response) bool {
	return req.Method == string(method)
}

// Parse implements strutils.Parser.
func (k *HTTPHeader) Parse(v string) error {
	split := strutils.SplitRune(v, '=')
	switch len(split) {
	case 1:
		split = append(split, "")
	case 2:
	default:
		return ErrInvalidHTTPHeaderFilter.Subject(v)
	}
	k.Key = split[0]
	k.Value = split[1]
	return nil
}

func (k *HTTPHeader) Fulfill(req *http.Request, res *http.Response) bool {
	wanted := k.Value
	// non canonical key matching
	got, ok := req.Header[k.Key]
	if wanted == "" {
		return ok
	}
	if !ok {
		return false
	}
	for _, v := range got {
		if strings.EqualFold(v, wanted) {
			return true
		}
	}
	return false
}

func (h Host) Fulfill(req *http.Request, res *http.Response) bool {
	return req.Host == string(h)
}

func (cidr *CIDR) Fulfill(req *http.Request, res *http.Response) bool {
	ip, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		ip = req.RemoteAddr
	}
	netIP := net.ParseIP(ip)
	if netIP == nil {
		return false
	}
	return cidr.Contains(netIP)
}
