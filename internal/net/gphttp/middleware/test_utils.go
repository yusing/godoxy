package middleware

import (
	"bytes"
	_ "embed"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"

	"github.com/bytedance/sonic"
	"github.com/yusing/godoxy/internal/common"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/goutils/http/reverseproxy"
)

//go:embed test_data/sample_headers.json
var testHeadersRaw []byte
var testHeaders http.Header

func init() {
	if !common.IsTest {
		return
	}
	tmp := map[string]string{}
	err := sonic.Unmarshal(testHeadersRaw, &tmp)
	if err != nil {
		panic(err)
	}
	testHeaders = http.Header{}
	for k, v := range tmp {
		testHeaders.Set(k, v)
	}
}

type requestRecorder struct {
	args *testArgs

	parent     http.RoundTripper
	headers    http.Header
	remoteAddr string
}

func newRequestRecorder(args *testArgs) *requestRecorder {
	return &requestRecorder{args: args}
}

func (rt *requestRecorder) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	rt.headers = req.Header
	rt.remoteAddr = req.RemoteAddr
	if rt.parent != nil {
		resp, err = rt.parent.RoundTrip(req)
	} else {
		resp = &http.Response{
			Status:        http.StatusText(rt.args.respStatus),
			StatusCode:    rt.args.respStatus,
			Header:        testHeaders,
			Body:          io.NopCloser(bytes.NewReader(rt.args.respBody)),
			ContentLength: int64(len(rt.args.respBody)),
			Request:       req,
			TLS:           req.TLS,
		}
	}
	if err != nil {
		return nil, err
	}
	maps.Copy(resp.Header, rt.args.respHeaders)
	return resp, nil
}

type TestResult struct {
	RequestHeaders  http.Header
	ResponseHeaders http.Header
	ResponseStatus  int
	RemoteAddr      string
	Data            []byte
}

type testArgs struct {
	middlewareOpt OptionsRaw
	upstreamURL   *nettypes.URL

	realRoundTrip bool

	reqURL    *nettypes.URL
	reqMethod string
	headers   http.Header
	body      []byte

	respHeaders http.Header
	respBody    []byte
	respStatus  int
}

func (args *testArgs) setDefaults() {
	if args.reqURL == nil {
		args.reqURL = nettypes.MustParseURL("https://example.com")
	}
	if args.reqMethod == "" {
		args.reqMethod = http.MethodGet
	}
	if args.upstreamURL == nil {
		args.upstreamURL = nettypes.MustParseURL("https://10.0.0.1:8443") // dummy url, no actual effect
	}
	if args.respHeaders == nil {
		args.respHeaders = http.Header{}
	}
	if args.respBody == nil {
		args.respBody = []byte("OK")
	}
	if args.respStatus == 0 {
		args.respStatus = http.StatusOK
	}
}

func (args *testArgs) bodyReader() io.Reader {
	if args.body != nil {
		return bytes.NewReader(args.body)
	}
	return nil
}

func newMiddlewareTest(middleware *Middleware, args *testArgs) (*TestResult, error) {
	if args == nil {
		args = new(testArgs)
	}
	args.setDefaults()

	mid, setOptErr := middleware.New(args.middlewareOpt)
	if setOptErr != nil {
		return nil, setOptErr
	}

	return newMiddlewaresTest([]*Middleware{mid}, args)
}

func newMiddlewaresTest(middlewares []*Middleware, args *testArgs) (*TestResult, error) {
	if args == nil {
		args = new(testArgs)
	}
	args.setDefaults()

	req := httptest.NewRequest(args.reqMethod, args.reqURL.String(), args.bodyReader())
	maps.Copy(req.Header, args.headers)

	w := httptest.NewRecorder()

	rr := newRequestRecorder(args)
	if args.realRoundTrip {
		rr.parent = http.DefaultTransport
	}

	rp := reverseproxy.NewReverseProxy("test", &args.upstreamURL.URL, rr)
	patchReverseProxy(rp, middlewares)
	rp.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return &TestResult{
		RequestHeaders:  rr.headers,
		ResponseHeaders: resp.Header,
		ResponseStatus:  resp.StatusCode,
		RemoteAddr:      rr.remoteAddr,
		Data:            data,
	}, nil
}
