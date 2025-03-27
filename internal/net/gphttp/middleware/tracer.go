package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/yusing/go-proxy/internal/utils/strutils"
)

type Tracer struct {
	name    string
	enabled bool
}

func _() {
	var _ MiddlewareWithTracer = &Tracer{}
}

func (t *Tracer) enableTrace() {
	t.enabled = true
}

func (t *Tracer) getTracer() *Tracer {
	return t
}

func (t *Tracer) SetParent(parent *Tracer) {
	if parent == nil {
		return
	}
	t.name = parent.name + "." + t.name
}

func (t *Tracer) addTrace(msg string) *Trace {
	return addTrace(&Trace{
		Time:    strutils.FormatTime(time.Now()),
		Caller:  t.name,
		Message: msg,
	})
}

func (t *Tracer) AddTracef(msg string, args ...any) *Trace {
	if !t.enabled {
		return nil
	}
	return t.addTrace(fmt.Sprintf(msg, args...))
}

func (t *Tracer) AddTraceRequest(msg string, req *http.Request) *Trace {
	if !t.enabled {
		return nil
	}
	return t.addTrace(msg).WithRequest(req)
}

func (t *Tracer) AddTraceResponse(msg string, resp *http.Response) *Trace {
	if !t.enabled {
		return nil
	}
	return t.addTrace(msg).WithResponse(resp)
}
