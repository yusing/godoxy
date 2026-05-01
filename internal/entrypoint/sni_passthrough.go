package entrypoint

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/pires/go-proxyproto"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog/log"
	acl "github.com/yusing/godoxy/internal/acl/types"
	autocert "github.com/yusing/godoxy/internal/autocert/types"
	netutils "github.com/yusing/godoxy/internal/net"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/types"
)

var errClientHelloRead = errors.New("client hello read")

const clientHelloTimeout = 5 * time.Second

type sniRouter struct {
	ep        *Entrypoint
	mu        sync.Mutex
	listeners *xsync.Map[string, *sniListener]
	routes    *xsync.Map[string, types.StreamRoute]
}

type sniListener struct {
	net.Listener
	router *sniRouter
	addr   string
	mu     sync.Mutex
	https  chan net.Conn
	closed bool
}

func newSNIRouter(ep *Entrypoint) *sniRouter {
	r := &sniRouter{
		ep:        ep,
		listeners: xsync.NewMap[string, *sniListener](),
		routes:    xsync.NewMap[string, types.StreamRoute](),
	}
	ep.task.OnCancel("sni_router", func() { _ = r.Close() })
	return r
}

func (r *sniRouter) AddRoute(route types.StreamRoute) error {
	if _, ok := route.Stream().(nettypes.ConnProxy); !ok {
		return fmt.Errorf("route %q stream does not support accepted connection proxying", route.Name())
	}
	if routeTerminatesTLS(route) && autocert.FromCtx(r.ep.task.Context()) == nil {
		return fmt.Errorf("route %q tls_termination requires an autocert provider", route.Name())
	}
	addr := sniListenAddr(route)
	if _, err := r.Listen(addr); err != nil {
		return err
	}
	r.routes.Store(sniRouteKey(addr, route.Key()), route)
	return nil
}

func (r *sniRouter) DelRoute(route types.StreamRoute) {
	r.routes.Delete(sniRouteKey(sniListenAddr(route), route.Key()))
}

func (r *sniRouter) Listen(addr string) (net.Listener, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if listener, ok := r.listeners.Load(addr); ok {
		return listener, nil
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	if r.ep.cfg.SupportProxyProtocol {
		ln = &proxyproto.Listener{Listener: ln}
	}
	if aclCfg := acl.FromCtx(r.ep.task.Context()); aclCfg != nil {
		ln = aclCfg.WrapTCP(ln)
	}
	listener := &sniListener{
		Listener: ln,
		router:   r,
		addr:     addr,
		https:    make(chan net.Conn, 128),
	}
	r.listeners.Store(addr, listener)
	go r.accept(addr, listener)
	return listener, nil
}

func (r *sniRouter) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var err error
	for addr, listener := range r.listeners.Range {
		err = errors.Join(err, listener.Close())
		r.listeners.Delete(addr)
	}
	return err
}

func (r *sniRouter) accept(addr string, listener *sniListener) {
	for {
		conn, err := listener.Listener.Accept()
		if err != nil {
			current, ok := r.listeners.Load(addr)
			if !ok || current != listener || errors.Is(err, net.ErrClosed) {
				return
			}
			log.Err(err).Msg("failed to accept sni-routed connection")
			continue
		}
		go r.handle(listener, conn)
	}
}

func (r *sniRouter) handle(listener *sniListener, conn net.Conn) {
	_ = conn.SetReadDeadline(time.Now().Add(clientHelloTimeout))
	serverName, replayConn, err := readClientHelloServerName(conn)
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil {
		r.forwardHTTPS(listener, replayConn)
		return
	}
	route, ok := r.match(listener, serverName)
	if !ok {
		r.forwardHTTPS(listener, replayConn)
		return
	}
	proxy := route.Stream().(nettypes.ConnProxy)
	if routeTerminatesTLS(route) {
		terminatedConn, err := r.terminateTLS(route.Task().Context(), replayConn)
		if err != nil {
			log.Debug().Err(err).Str("server_name", serverName).Msg("failed to terminate tls for tcp route")
			_ = replayConn.Close()
			return
		}
		replayConn = terminatedConn
	}
	proxy.ProxyConn(route.Task().Context(), replayConn)
}

func (r *sniRouter) forwardHTTPS(listener *sniListener, conn net.Conn) {
	listener.forwardHTTPS(conn)
}

func (r *sniRouter) terminateTLS(ctx context.Context, conn net.Conn) (net.Conn, error) {
	provider := autocert.FromCtx(r.ep.task.Context())
	tlsConn := tls.Server(conn, &tls.Config{GetCertificate: provider.GetCert, MinVersion: tls.VersionTLS12})
	_ = conn.SetReadDeadline(time.Now().Add(clientHelloTimeout))
	err := tlsConn.HandshakeContext(ctx)
	_ = conn.SetReadDeadline(time.Time{})
	return tlsConn, err
}

func (r *sniRouter) match(listener *sniListener, serverName string) (types.StreamRoute, bool) {
	key, ok := matchSNI(serverName, r.ep.findRouteKeyFunc, func(key string) bool {
		_, ok := r.routes.Load(sniRouteKey(listener.addr, key))
		return ok
	})
	if !ok {
		return nil, false
	}
	return r.routes.Load(sniRouteKey(listener.addr, key))
}

func matchSNI(serverName string, findKey findRouteKeyFunc, exists func(string) bool) (string, bool) {
	serverName = normalizeSNIName(serverName)
	if serverName == "" {
		return "", false
	}
	return findKey(serverName, exists)
}

func asSNIRoute(route types.StreamRoute) bool {
	listenURL := route.ListenURL()
	switch listenURL.Scheme {
	case "tcp", "tcp4", "tcp6":
		return netutils.IsSharedHTTPSListenAddr(listenURL.Host)
	default:
		return false
	}
}

func sniListenAddr(route types.StreamRoute) string {
	return netutils.SharedHTTPSListenAddr(route.ListenURL().Host)
}

func routeTerminatesTLS(route types.StreamRoute) bool {
	terminatingRoute, ok := route.(interface{ TerminatesTLS() bool })
	return ok && terminatingRoute.TerminatesTLS()
}

func (l *sniListener) Accept() (net.Conn, error) {
	conn, ok := <-l.https
	if !ok {
		return nil, net.ErrClosed
	}
	return conn, nil
}

func (l *sniListener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	err := l.Listener.Close()
	close(l.https)
	if l.router != nil {
		l.router.listeners.Delete(l.addr)
	}
	return err
}

func (l *sniListener) forwardHTTPS(conn net.Conn) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		_ = conn.Close()
		return
	}
	select {
	case l.https <- conn:
	default:
		_ = conn.Close()
	}
}

func normalizeSNIName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name, _, _ = strings.Cut(name, ":")
	return name
}

func sniRouteKey(addr, name string) string {
	return addr + "\x00" + normalizeSNIName(name)
}

type clientHelloReadConn struct {
	net.Conn
	reader io.Reader
}

func (c *clientHelloReadConn) Read(p []byte) (int, error)  { return c.reader.Read(p) }
func (c *clientHelloReadConn) Write(p []byte) (int, error) { return len(p), nil }

type replayConn struct {
	net.Conn
	reader io.Reader
}

func (c *replayConn) Read(p []byte) (int, error) { return c.reader.Read(p) }

func readClientHelloServerName(conn net.Conn) (string, net.Conn, error) {
	var captured bytes.Buffer
	var serverName string
	tlsConn := tls.Server(&clientHelloReadConn{Conn: conn, reader: io.TeeReader(conn, &captured)}, &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			serverName = normalizeSNIName(hello.ServerName)
			return nil, errClientHelloRead
		},
	})
	err := tlsConn.Handshake()
	replayed := &replayConn{Conn: conn, reader: io.MultiReader(bytes.NewReader(captured.Bytes()), conn)}
	if errors.Is(err, errClientHelloRead) {
		return serverName, replayed, nil
	}
	return serverName, replayed, err
}
