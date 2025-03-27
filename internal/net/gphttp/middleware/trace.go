package middleware

import (
	"net/http"
	"sync"

	"github.com/yusing/go-proxy/internal/net/gphttp/httpheaders"
)

type (
	Trace struct {
		Time        string            `json:"time,omitempty"`
		Caller      string            `json:"caller,omitempty"`
		URL         string            `json:"url,omitempty"`
		Message     string            `json:"msg"`
		ReqHeaders  map[string]string `json:"req_headers,omitempty"`
		RespHeaders map[string]string `json:"resp_headers,omitempty"`
		RespStatus  int               `json:"resp_status,omitempty"`
		Additional  map[string]any    `json:"additional,omitempty"`
	}
	Traces []*Trace
)

var (
	traces   = make(Traces, 0)
	tracesMu sync.Mutex
)

const MaxTraceNum = 100

func GetAllTrace() []*Trace {
	return traces
}

func (tr *Trace) WithRequest(req *http.Request) *Trace {
	if tr == nil {
		return nil
	}
	tr.URL = req.RequestURI
	tr.ReqHeaders = httpheaders.HeaderToMap(req.Header)
	return tr
}

func (tr *Trace) WithResponse(resp *http.Response) *Trace {
	if tr == nil {
		return nil
	}
	tr.URL = resp.Request.RequestURI
	tr.ReqHeaders = httpheaders.HeaderToMap(resp.Request.Header)
	tr.RespHeaders = httpheaders.HeaderToMap(resp.Header)
	tr.RespStatus = resp.StatusCode
	return tr
}

func (tr *Trace) With(what string, additional any) *Trace {
	if tr == nil {
		return nil
	}

	if tr.Additional == nil {
		tr.Additional = map[string]any{}
	}
	tr.Additional[what] = additional
	return tr
}

func (tr *Trace) WithError(err error) *Trace {
	if tr == nil {
		return nil
	}

	if tr.Additional == nil {
		tr.Additional = map[string]any{}
	}
	tr.Additional["error"] = err.Error()
	return tr
}

func addTrace(t *Trace) *Trace {
	tracesMu.Lock()
	defer tracesMu.Unlock()
	if len(traces) > MaxTraceNum {
		traces = traces[1:]
	}
	traces = append(traces, t)
	return t
}
