// Copyright 2011 The Go Authors.
// Modified from the Go project under the a BSD-style License (https://cs.opensource.google/go/go/+/refs/tags/go1.23.1:src/net/http/httputil/reverseproxy.go)
// https://cs.opensource.google/go/go/+/master:LICENSE

package reverseproxy

// This is a small mod on net/http/httputil/reverseproxy.go
// that boosts performance in some cases
// and compatible to other modules of this project
// Copyright (c) 2024 yusing

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/textproto"
	"net/url"
	"strings"
	"sync"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/internal/logging/accesslog"
	"github.com/yusing/go-proxy/internal/net/gphttp/httpheaders"
	nettypes "github.com/yusing/go-proxy/internal/net/types"
	U "github.com/yusing/go-proxy/internal/utils"
	"golang.org/x/net/http/httpguts"
)

// A ProxyRequest contains a request to be rewritten by a [ReverseProxy].
type ProxyRequest struct {
	// In is the request received by the proxy.
	// The Rewrite function must not modify In.
	In *http.Request

	// Out is the request which will be sent by the proxy.
	// The Rewrite function may modify or replace this request.
	// Hop-by-hop headers are removed from this request
	// before Rewrite is called.
	Out *http.Request
}

// SetXForwarded sets the X-Forwarded-For, X-Forwarded-Host, and
// X-Forwarded-Proto headers of the outbound request.
//
//   - The X-Forwarded-For header is set to the client IP address.
//   - The X-Forwarded-Host header is set to the host name requested
//     by the client.
//   - The X-Forwarded-Proto header is set to "http" or "https", depending
//     on whether the inbound request was made on a TLS-enabled connection.
//
// If the outbound request contains an existing X-Forwarded-For header,
// SetXForwarded appends the client IP address to it. To append to the
// inbound request's X-Forwarded-For header (the default behavior of
// [ReverseProxy] when using a Director function), copy the header
// from the inbound request before calling SetXForwarded:
//
//	rewriteFunc := func(r *httputil.ProxyRequest) {
//		r.Out.Header["X-Forwarded-For"] = r.In.Header["X-Forwarded-For"]
//		r.SetXForwarded()
//	}

// ReverseProxy is an HTTP Handler that takes an incoming request and
// sends it to another server, proxying the response back to the
// client.
//
// 1xx responses are forwarded to the client if the underlying
// transport supports ClientTrace.Got1xxResponse.
type ReverseProxy struct {
	zerolog.Logger

	// The transport used to perform proxy requests.
	Transport http.RoundTripper

	// ModifyResponse is an optional function that modifies the
	// Response from the backend. It is called if the backend
	// returns a response at all, with any HTTP status code.
	// If the backend is unreachable, the optional ErrorHandler is
	// called before ModifyResponse.
	//
	// If ModifyResponse returns an error, ErrorHandler is called
	// with its error value. If ErrorHandler is nil, its default
	// implementation is used.
	ModifyResponse func(*http.Response) error
	AccessLogger   *accesslog.AccessLogger

	HandlerFunc http.HandlerFunc

	TargetName string
	TargetURL  *nettypes.URL
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}

func joinURLPath(a, b *url.URL) (path, rawpath string) {
	if a.RawPath == "" && b.RawPath == "" {
		return singleJoiningSlash(a.Path, b.Path), ""
	}
	// Same as singleJoiningSlash, but uses EscapedPath to determine
	// whether a slash should be added
	apath := a.EscapedPath()
	bpath := b.EscapedPath()

	aslash := strings.HasSuffix(apath, "/")
	bslash := strings.HasPrefix(bpath, "/")

	switch {
	case aslash && bslash:
		return a.Path + b.Path[1:], apath + bpath[1:]
	case !aslash && !bslash:
		return a.Path + "/" + b.Path, apath + "/" + bpath
	}
	return a.Path + b.Path, apath + bpath
}

// NewReverseProxy returns a new [ReverseProxy] that routes
// URLs to the scheme, host, and base path provided in target. If the
// target's path is "/base" and the incoming request was for "/dir",
// the target request will be for /base/dir.
func NewReverseProxy(name string, target *nettypes.URL, transport http.RoundTripper) *ReverseProxy {
	if transport == nil {
		panic("nil transport")
	}
	rp := &ReverseProxy{
		Logger:     log.With().Str("name", name).Logger(),
		Transport:  transport,
		TargetName: name,
		TargetURL:  target,
	}
	rp.HandlerFunc = rp.handler
	return rp
}

func (p *ReverseProxy) rewriteRequestURL(req *http.Request) {
	targetQuery := p.TargetURL.RawQuery
	req.URL.Scheme = p.TargetURL.Scheme
	req.URL.Host = p.TargetURL.Host
	req.URL.Path, req.URL.RawPath = joinURLPath(&p.TargetURL.URL, req.URL)
	if targetQuery == "" || req.URL.RawQuery == "" {
		req.URL.RawQuery = targetQuery + req.URL.RawQuery
	} else {
		req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
	}
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func (p *ReverseProxy) errorHandler(rw http.ResponseWriter, r *http.Request, err error, writeHeader bool) {
	reqURL := r.Host + r.URL.Path
	switch {
	case errors.Is(err, context.Canceled),
		errors.Is(err, io.EOF),
		errors.Is(err, context.DeadlineExceeded):
		log.Debug().Err(err).Str("url", reqURL).Msg("http proxy error")
	default:
		var recordErr tls.RecordHeaderError
		if errors.As(err, &recordErr) {
			log.Error().
				Str("url", reqURL).
				Msgf(`scheme was likely misconfigured as https,
						try setting "proxy.%s.scheme" back to "http"`, p.TargetName)
			log.Err(err).Msg("underlying error")
		} else {
			log.Err(err).Str("url", reqURL).Msg("http proxy error")
		}
	}

	if writeHeader {
		rw.WriteHeader(http.StatusInternalServerError)
	}
	if p.AccessLogger != nil {
		p.AccessLogger.LogError(r, err)
	}
}

// modifyResponse conditionally runs the optional ModifyResponse hook
// and reports whether the request should proceed.
func (p *ReverseProxy) modifyResponse(rw http.ResponseWriter, res *http.Response, origReq, req *http.Request) bool {
	if p.ModifyResponse == nil {
		return true
	}
	res.Request = origReq
	err := p.ModifyResponse(res)
	res.Request = req
	if err != nil {
		res.Body.Close()
		p.errorHandler(rw, req, err, true)
		return false
	}
	return true
}

func (p *ReverseProxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	p.HandlerFunc(rw, req)
}

func (p *ReverseProxy) handler(rw http.ResponseWriter, req *http.Request) {
	transport := p.Transport

	ctx := req.Context()
	if ctx.Done() != nil {
		// CloseNotifier predates context.Context, and has been
		// entirely superseded by it. If the request contains
		// a Context that carries a cancellation signal, don't
		// bother spinning up a goroutine to watch the CloseNotify
		// channel (if any).
		//
		// If the request Context has a nil Done channel (which
		// means it is either context.Background, or a custom
		// Context implementation with no cancellation signal),
		// then consult the CloseNotifier if available.
	} else if cn, ok := rw.(http.CloseNotifier); ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		defer cancel()
		notifyChan := cn.CloseNotify()
		go func() {
			select {
			case <-notifyChan:
				cancel()
			case <-ctx.Done():
			}
		}()
	}

	outreq := req.Clone(ctx)
	if req.ContentLength == 0 {
		outreq.Body = nil // Issue 16036: nil Body for http.Transport retries
	}
	if outreq.Body != nil {
		// Reading from the request body after returning from a handler is not
		// allowed, and the RoundTrip goroutine that reads the Body can outlive
		// this handler. This can lead to a crash if the handler panics (see
		// Issue 46866). Although calling Close doesn't guarantee there isn't
		// any Read in flight after the handle returns, in practice it's safe to
		// read after closing it.
		defer outreq.Body.Close()
	}
	if outreq.Header == nil {
		outreq.Header = make(http.Header) // Issue 33142: historical behavior was to always allocate
	}

	p.rewriteRequestURL(outreq)
	outreq.Close = false

	reqUpType := httpheaders.UpgradeType(outreq.Header)
	if !IsPrint(reqUpType) {
		p.errorHandler(rw, req, fmt.Errorf("client tried to switch to invalid protocol %q", reqUpType), true)
		return
	}

	outreq.Header.Del("Forwarded")
	httpheaders.RemoveHopByHopHeaders(outreq.Header)

	// Issue 21096: tell backend applications that care about trailer support
	// that we support trailers. (We do, but we don't go out of our way to
	// advertise that unless the incoming client request thought it was worth
	// mentioning.) Note that we look at req.Header, not outreq.Header, since
	// the latter has passed through removeHopByHopHeaders.
	if httpguts.HeaderValuesContainsToken(req.Header["Te"], "trailers") {
		outreq.Header.Set("Te", "trailers")
	}

	// After stripping all the hop-by-hop connection headers above, add back any
	// necessary for protocol upgrades, such as for websockets.
	if reqUpType != "" {
		outreq.Header.Set("Connection", "Upgrade")
		outreq.Header.Set("Upgrade", reqUpType)

		if strings.EqualFold(reqUpType, "websocket") {
			cleanWebsocketHeaders(outreq)
		}
	}

	// If we aren't the first proxy retain prior
	// X-Forwarded-For information as a comma+space
	// separated list and fold multiple headers into one.
	prior, ok := outreq.Header[httpheaders.HeaderXForwardedFor]
	omit := ok && prior == nil // Issue 38079: nil now means don't populate the header

	if !omit {
		xff, _, err := net.SplitHostPort(req.RemoteAddr)
		if err != nil {
			xff = req.RemoteAddr
		}
		if len(prior) > 0 {
			xff = strings.Join(prior, ", ") + ", " + xff
		}
		outreq.Header.Set(httpheaders.HeaderXForwardedFor, xff)
	}

	var reqScheme string
	if req.TLS != nil || req.Header.Get("X-Forwarded-Proto") == "https" {
		reqScheme = "https"
	} else {
		reqScheme = "http"
	}

	outreq.Header.Set(httpheaders.HeaderXForwardedMethod, req.Method)
	outreq.Header.Set(httpheaders.HeaderXForwardedProto, reqScheme)
	outreq.Header.Set(httpheaders.HeaderXForwardedHost, req.Host)
	outreq.Header.Set(httpheaders.HeaderXForwardedURI, req.RequestURI)

	if _, ok := outreq.Header["User-Agent"]; !ok {
		// If the outbound request doesn't have a User-Agent header set,
		// don't send the default Go HTTP client User-Agent.
		outreq.Header.Set("User-Agent", "")
	}

	var (
		roundTripMutex sync.Mutex
		roundTripDone  bool
	)
	trace := &httptrace.ClientTrace{
		Got1xxResponse: func(code int, header textproto.MIMEHeader) error {
			roundTripMutex.Lock()
			defer roundTripMutex.Unlock()
			if roundTripDone {
				// If RoundTrip has returned, don't try to further modify
				// the ResponseWriter's header map.
				return nil
			}
			h := rw.Header()
			copyHeader(h, http.Header(header))
			rw.WriteHeader(code)

			// Clear headers, it's not automatically done by ResponseWriter.WriteHeader() for 1xx responses
			clear(h)
			return nil
		},
	}
	outreq = outreq.WithContext(httptrace.WithClientTrace(outreq.Context(), trace)) //nolint:contextcheck

	res, err := transport.RoundTrip(outreq)

	roundTripMutex.Lock()
	roundTripDone = true
	roundTripMutex.Unlock()
	if err != nil {
		p.errorHandler(rw, outreq, err, false)
		res = &http.Response{
			Status:     http.StatusText(http.StatusBadGateway),
			StatusCode: http.StatusBadGateway,
			Proto:      req.Proto,
			ProtoMajor: req.ProtoMajor,
			ProtoMinor: req.ProtoMinor,
			Header:     http.Header{},
			Body:       io.NopCloser(bytes.NewReader([]byte("Origin server is not reachable."))),
			Request:    req,
			TLS:        req.TLS,
		}
	}

	if p.AccessLogger != nil {
		defer func() {
			p.AccessLogger.Log(req, res)
		}()
	}

	httpheaders.RemoveServiceHeaders(res.Header)

	// Deal with 101 Switching Protocols responses: (WebSocket, h2c, etc)
	if res.StatusCode == http.StatusSwitchingProtocols {
		if !p.modifyResponse(rw, res, req, outreq) {
			return
		}
		p.handleUpgradeResponse(rw, outreq, res)
		return
	}

	httpheaders.RemoveHopByHopHeaders(res.Header)

	if !p.modifyResponse(rw, res, req, outreq) {
		return
	}

	copyHeader(rw.Header(), res.Header)

	// The "Trailer" header isn't included in the Transport's response,
	// at least for *http.Transport. Build it up from Trailer.
	announcedTrailers := len(res.Trailer)
	if announcedTrailers > 0 {
		trailerKeys := make([]string, 0, len(res.Trailer))
		for k := range res.Trailer {
			trailerKeys = append(trailerKeys, k)
		}
		rw.Header().Add("Trailer", strings.Join(trailerKeys, ", "))
	}

	rw.WriteHeader(res.StatusCode)

	err = U.CopyCloseWithContext(ctx, rw, res.Body, int(res.ContentLength)) // close now, instead of defer, to populate res.Trailer
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			p.errorHandler(rw, req, err, false)
		}
		return
	}

	if len(res.Trailer) > 0 {
		// Force chunking if we saw a response trailer.
		// This prevents net/http from calculating the length for short
		// bodies and adding a Content-Length.
		http.NewResponseController(rw).Flush()
	}

	if len(res.Trailer) == announcedTrailers {
		copyHeader(rw.Header(), res.Trailer)
		return
	}

	for k, vv := range res.Trailer {
		k = http.TrailerPrefix + k
		for _, v := range vv {
			rw.Header().Add(k, v)
		}
	}
}

// reference: https://github.com/traefik/traefik/blob/master/pkg/proxy/httputil/proxy.go
// https://tools.ietf.org/html/rfc6455#page-20
func cleanWebsocketHeaders(req *http.Request) {
	req.Header["Sec-WebSocket-Key"] = req.Header["Sec-Websocket-Key"]
	delete(req.Header, "Sec-Websocket-Key")

	req.Header["Sec-WebSocket-Extensions"] = req.Header["Sec-Websocket-Extensions"]
	delete(req.Header, "Sec-Websocket-Extensions")

	req.Header["Sec-WebSocket-Accept"] = req.Header["Sec-Websocket-Accept"]
	delete(req.Header, "Sec-Websocket-Accept")

	req.Header["Sec-WebSocket-Protocol"] = req.Header["Sec-Websocket-Protocol"]
	delete(req.Header, "Sec-Websocket-Protocol")

	req.Header["Sec-WebSocket-Version"] = req.Header["Sec-Websocket-Version"]
	delete(req.Header, "Sec-Websocket-Version")
}

func (p *ReverseProxy) handleUpgradeResponse(rw http.ResponseWriter, req *http.Request, res *http.Response) {
	reqUpType := httpheaders.UpgradeType(req.Header)
	resUpType := httpheaders.UpgradeType(res.Header)
	if !IsPrint(resUpType) { // We know reqUpType is ASCII, it's checked by the caller.
		p.errorHandler(rw, req, fmt.Errorf("backend tried to switch to invalid protocol %q", resUpType), true)
		return
	}
	if !strings.EqualFold(reqUpType, resUpType) {
		p.errorHandler(rw, req, fmt.Errorf("backend tried to switch protocol %q when %q was requested", resUpType, reqUpType), true)
		return
	}

	backConn, ok := res.Body.(io.ReadWriteCloser)
	if !ok {
		p.errorHandler(rw, req, errors.New("internal error: 101 switching protocols response with non-writable body"), true)
		return
	}

	rc := http.NewResponseController(rw)
	conn, brw, hijackErr := rc.Hijack()
	if errors.Is(hijackErr, http.ErrNotSupported) {
		p.errorHandler(rw, req, fmt.Errorf("can't switch protocols using non-Hijacker ResponseWriter type %T", rw), true)
		return
	}

	backConnCloseCh := make(chan bool)
	go func() {
		// Ensure that the cancellation of a request closes the backend.
		// See issue https://golang.org/issue/35559.
		select {
		case <-req.Context().Done():
		case <-backConnCloseCh:
		}
		backConn.Close()
	}()
	defer close(backConnCloseCh)

	if hijackErr != nil {
		p.errorHandler(rw, req, fmt.Errorf("hijack failed on protocol switch: %w", hijackErr), true)
		return
	}
	defer conn.Close()

	copyHeader(rw.Header(), res.Header)

	res.Header = rw.Header()
	res.Body = nil // so res.Write only writes the headers; we have res.Body in backConn above
	if err := res.Write(brw); err != nil {
		//nolint:errorlint
		p.errorHandler(rw, req, fmt.Errorf("response write: %s", err), true)
		return
	}
	if err := brw.Flush(); err != nil {
		//nolint:errorlint
		p.errorHandler(rw, req, fmt.Errorf("response flush: %s", err), true)
		return
	}

	bdp := U.NewBidirectionalPipe(req.Context(), conn, backConn)
	//nolint:errcheck
	bdp.Start()
}

func IsPrint(s string) bool {
	for _, r := range s {
		if r < ' ' || r > '~' {
			return false
		}
	}
	return true
}
