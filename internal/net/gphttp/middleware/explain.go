package middleware

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"

	"github.com/yusing/godoxy/internal/logging/accesslog"
)

// ExplainResult is the outcome of one middleware in a dry run.
type ExplainResult string

const (
	// ExplainPassed: the middleware ran and let the request through.
	ExplainPassed ExplainResult = "passed"
	// ExplainDenied: the middleware ran and answered with a 4xx/5xx.
	ExplainDenied ExplainResult = "denied"
	// ExplainResponded: the middleware ran and answered itself (e.g. a redirect).
	ExplainResponded ExplainResult = "responded"
	// ExplainBypassed: a bypass rule matched, the middleware did not run.
	ExplainBypassed ExplainResult = "bypassed"
	// ExplainSkipped: the middleware does not apply (e.g. its route entry was
	// promoted into the entrypoint chain).
	ExplainSkipped ExplainResult = "skipped"
	// ExplainNotEvaluated: the middleware has side effects or needs a live
	// client (auth, CrowdSec, rate limit), so the dry run does not run it.
	ExplainNotEvaluated ExplainResult = "not_evaluated"
)

// ExplainStep is one middleware's outcome in a dry run.
type ExplainStep struct {
	// Middleware is the configured name, e.g. "entrypoint[2]" or "cidrWhitelist".
	Middleware string `json:"middleware"`
	// Type is the middleware implementation, e.g. "cidrWhitelist".
	Type   string        `json:"type"`
	Result ExplainResult `json:"result"`
	Detail string        `json:"detail,omitempty"`
	// ClientIP is set when this step changed the client IP (real IP middlewares).
	ClientIP string `json:"client_ip,omitempty"`
} // @name MiddlewareExplainStep

// Explain dry-runs the request phase of m on r. Middlewares without side
// effects (real IP, cidr_whitelist, redirect, header modifiers) run for real
// against a recorder; the others are reported as not evaluated, after their
// bypass rules are checked. It returns the steps and whether the request
// would reach the next handler. r is modified like a real request would be
// (e.g. RemoteAddr after real IP middlewares).
func (m *Middleware) Explain(r *http.Request) (steps []ExplainStep, proceed bool) {
	mod, ok := m.impl.(RequestModifier)
	if !ok {
		return nil, true
	}
	return explainModifier(m.name, mod, r, steps)
}

func explainModifier(name string, mod RequestModifier, r *http.Request, steps []ExplainStep) ([]ExplainStep, bool) {
	if chain, ok := mod.(*middlewareChain); ok {
		for i, before := range chain.befores {
			var proceed bool
			if steps, proceed = explainModifier(chain.beforeNames[i], before, r, steps); !proceed {
				return steps, false
			}
		}
		return steps, true
	}

	impl := mod
	if bypass, ok := mod.(*checkBypass); ok {
		impl = bypass.modReq
		step := ExplainStep{Middleware: name, Type: typeName(impl)}
		switch {
		case isRouteMiddlewareConsumed(r, bypass.name):
			step.Result = ExplainSkipped
			step.Detail = "bypass rules promoted into the entrypoint chain"
			return append(steps, step), true
		case impl == nil:
			return steps, true
		case bypass.shouldModReqBypass(httptest.NewRecorder(), r):
			step.Result = ExplainBypassed
			step.Detail = "a bypass rule matched"
			return append(steps, step), true
		}
		if inner, ok := impl.(*middlewareChain); ok {
			return explainModifier(name, inner, r, steps)
		}
	}

	step := ExplainStep{Middleware: name, Type: typeName(impl)}
	if !isDryRunSafe(impl) {
		step.Result = ExplainNotEvaluated
		step.Detail = "has side effects or needs a live client; result depends on the real request"
		return append(steps, step), true
	}

	clientIPBefore := hostOf(r.RemoteAddr)
	rec := httptest.NewRecorder()
	proceed := impl.before(rec, r)
	if ip := hostOf(r.RemoteAddr); ip != clientIPBefore {
		step.ClientIP = ip
	}
	switch {
	case proceed:
		step.Result = ExplainPassed
	case rec.Code >= http.StatusBadRequest:
		step.Result = ExplainDenied
		step.Detail = fmt.Sprintf("HTTP %d", rec.Code)
		if info, ok := accesslog.RequestInfoOf(r); ok && info.DenyReason != "" {
			step.Detail += ": " + info.DenyReason
		}
	default:
		step.Result = ExplainResponded
		step.Detail = fmt.Sprintf("HTTP %d", rec.Code)
		if loc := rec.Header().Get("Location"); loc != "" {
			step.Detail += " " + loc
		}
	}
	return append(steps, step), proceed
}

// isDryRunSafe reports whether running impl on a synthetic request has no
// effect outside that request.
func isDryRunSafe(impl RequestModifier) bool {
	switch impl.(type) {
	case *realIP, *cloudflareRealIP, *cidrWhitelist, *redirectHTTP,
		*setXForwarded, *hideXForwarded, *modifyRequest:
		return true
	default:
		return false
	}
}

func typeName(impl any) string {
	if impl == nil {
		return ""
	}
	t := reflect.TypeOf(impl)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Name()
}

func hostOf(addr string) string {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}
