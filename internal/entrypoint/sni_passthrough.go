package entrypoint

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pires/go-proxyproto"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog/log"
	acl "github.com/yusing/godoxy/internal/acl/types"
	"github.com/yusing/godoxy/internal/common"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/types"
)

var errClientHelloCaptured = errors.New("client hello captured")

const defaultClientHelloTimeout = 5 * time.Second

var clientHelloTimeout = defaultClientHelloTimeout

type sniPassthroughManager struct {
	ep    *Entrypoint
	muxes *xsync.Map[string, *sniPassthroughMux]
}

func newSNIPassthroughManager(ep *Entrypoint) *sniPassthroughManager {
	return &sniPassthroughManager{
		ep:    ep,
		muxes: xsync.NewMap[string, *sniPassthroughMux](),
	}
}

func asSNIPassthroughRoute(route types.StreamRoute) (types.StreamRoute, bool) {
	if routeListensOnHTTPSAddr(route.ListenURL()) {
		return route, true
	}
	return nil, false
}

func routeListensOnHTTPSAddr(listenURL *nettypes.URL) bool {
	if listenURL == nil {
		return false
	}
	switch listenURL.Scheme {
	case "tcp", "tcp4", "tcp6":
	default:
		return false
	}
	return listenAddrMatchesHTTPSAddr(listenURL.Host)
}

func listenAddrMatchesHTTPSAddr(addr string) bool {
	if addr == common.ProxyHTTPSAddr {
		return true
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil || port != strconv.Itoa(common.ProxyHTTPSPort) {
		return false
	}
	if common.ProxyHTTPSHost != "" {
		return host == common.ProxyHTTPSHost
	}
	return host == "" || host == "0.0.0.0" || host == "::"
}

func (m *sniPassthroughManager) HTTPSListener(addr string) (net.Listener, bool, error) {
	mux, err := m.getOrStart(newSNIListenKeyForAddr(addr))
	if err != nil {
		return nil, false, err
	}
	return mux.httpsListener(), true, nil
}

func (m *sniPassthroughManager) AddRoute(route types.StreamRoute) error {
	proxy, ok := route.Stream().(nettypes.ConnProxy)
	if !ok {
		return fmt.Errorf("route %q stream does not support accepted connection proxying", route.Name())
	}
	mux, err := m.getOrStart(newSNIListenKeyForAddr(common.ProxyHTTPSAddr))
	if err != nil {
		return err
	}
	return mux.addRoute(route.Key(), proxy, route.Task().Context())
}

func (m *sniPassthroughManager) DelRoute(route types.StreamRoute) {
	if mux, ok := m.muxes.Load(newSNIListenKeyForAddr(common.ProxyHTTPSAddr).String()); ok {
		mux.delRoute(route.Key())
	}
}

type sniListenKey struct {
	network string
	addr    string
}

func newSNIListenKey(network, addr string) sniListenKey {
	if network == "" {
		network = listenNetworkForAddr(addr)
	}
	return sniListenKey{network: network, addr: addr}
}

func newSNIListenKeyForAddr(addr string) sniListenKey {
	return newSNIListenKey(listenNetworkForAddr(addr), addr)
}

func listenNetworkForAddr(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "tcp"
	}
	_ = port
	if host == "" {
		return "tcp"
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return "tcp"
	}
	if ip.To4() == nil {
		return "tcp6"
	}
	return "tcp4"
}

func (k sniListenKey) String() string {
	return k.network + "|" + k.addr
}

func (m *sniPassthroughManager) getOrStart(key sniListenKey) (*sniPassthroughMux, error) {
	var startErr error
	mux, _ := m.muxes.LoadOrCompute(key.String(), func() (*sniPassthroughMux, bool) {
		created, err := newSNIPassthroughMux(m.ep, key)
		if err != nil {
			startErr = err
			return nil, true
		}
		created.start()
		return created, false
	})
	if startErr != nil {
		return nil, startErr
	}
	return mux, nil
}

type sniPassthroughMux struct {
	ep      *Entrypoint
	key     sniListenKey
	base    net.Listener
	httpsMu sync.Mutex
	https   *queuedListener
	routes  *xsync.Map[string, *sniPassthroughTarget]
	closed  atomic.Bool
}

type sniPassthroughTarget struct {
	key   string
	proxy nettypes.ConnProxy
	ctx   context.Context
}

func newSNIPassthroughMux(ep *Entrypoint, key sniListenKey) (*sniPassthroughMux, error) {
	base, err := net.Listen(key.network, key.addr)
	if err != nil {
		return nil, err
	}
	if ep.cfg.SupportProxyProtocol {
		base = &proxyproto.Listener{Listener: base}
	}
	if aclCfg := acl.FromCtx(ep.task.Context()); aclCfg != nil {
		base = aclCfg.WrapTCP(base)
	}
	mux := &sniPassthroughMux{
		ep:     ep,
		key:    key,
		base:   base,
		routes: xsync.NewMap[string, *sniPassthroughTarget](),
	}
	ep.task.OnCancel("sni_passthrough_mux:"+key.String(), func() {
		mux.close()
	})
	return mux, nil
}

func (m *sniPassthroughMux) start() {
	go m.acceptLoop(m.ep.task.Context())
}

func (m *sniPassthroughMux) httpsListener() net.Listener {
	m.httpsMu.Lock()
	defer m.httpsMu.Unlock()
	if m.https == nil || m.https.IsClosed() {
		m.https = newQueuedListener(m.base.Addr())
	}
	return m.https
}

func (m *sniPassthroughMux) existingHTTPSListener() *queuedListener {
	m.httpsMu.Lock()
	defer m.httpsMu.Unlock()
	if m.https == nil || m.https.IsClosed() {
		return nil
	}
	return m.https
}

func (m *sniPassthroughMux) addRoute(key string, proxy nettypes.ConnProxy, ctx context.Context) error {
	key = normalizeSNIName(key)
	if key == "" {
		return errors.New("sni passthrough route requires a non-empty alias")
	}
	target := &sniPassthroughTarget{key: key, proxy: proxy, ctx: ctx}
	m.routes.Store(key, target)
	return nil
}

func (m *sniPassthroughMux) delRoute(key string) {
	m.routes.Delete(normalizeSNIName(key))
}

func (m *sniPassthroughMux) acceptLoop(ctx context.Context) {
	for {
		conn, err := m.base.Accept()
		if err != nil {
			if m.closed.Load() || errors.Is(err, net.ErrClosed) || errors.Is(ctx.Err(), context.Canceled) {
				return
			}
			log.Err(err).Str("listener", m.key.String()).Msg("failed to accept sni passthrough connection")
			continue
		}
		go m.handleConn(ctx, conn)
	}
}

func (m *sniPassthroughMux) handleConn(ctx context.Context, conn net.Conn) {
	if clientHelloTimeout > 0 {
		_ = conn.SetReadDeadline(time.Now().Add(clientHelloTimeout))
	}
	serverName, replayConn, err := readClientHelloServerName(conn)
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil {
		log.Debug().Err(err).Str("listener", m.key.String()).Msg("failed to read client hello; forwarding to https")
		m.forwardHTTPS(replayConn)
		return
	}
	if target, ok := m.matchRoute(serverName); ok {
		target.proxy.ProxyConn(target.ctx, replayConn)
		return
	}
	m.forwardHTTPS(replayConn)
}

func (m *sniPassthroughMux) forwardHTTPS(conn net.Conn) {
	listener := m.existingHTTPSListener()
	if listener == nil {
		_ = conn.Close()
		return
	}
	if err := listener.Enqueue(conn); err != nil {
		_ = conn.Close()
	}
}

func (m *sniPassthroughMux) close() {
	if m.closed.Swap(true) {
		return
	}
	_ = m.base.Close()
	m.httpsMu.Lock()
	if m.https != nil {
		_ = m.https.Close()
	}
	m.httpsMu.Unlock()
}

type queuedListener struct {
	addr     net.Addr
	ch       chan net.Conn
	closedMu sync.Mutex
	closed   bool
}

func newQueuedListener(addr net.Addr) *queuedListener {
	return &queuedListener{addr: addr, ch: make(chan net.Conn, 128)}
}

func (l *queuedListener) Accept() (net.Conn, error) {
	conn, ok := <-l.ch
	if !ok {
		return nil, net.ErrClosed
	}
	return conn, nil
}

func (l *queuedListener) Close() error {
	l.closedMu.Lock()
	if l.closed {
		l.closedMu.Unlock()
		return nil
	}
	l.closed = true
	close(l.ch)
	l.closedMu.Unlock()
	for conn := range l.ch {
		_ = conn.Close()
	}
	return nil
}

func (l *queuedListener) Addr() net.Addr { return l.addr }
func (l *queuedListener) IsClosed() bool {
	l.closedMu.Lock()
	defer l.closedMu.Unlock()
	return l.closed
}

func (l *queuedListener) Enqueue(conn net.Conn) error {
	l.closedMu.Lock()
	defer l.closedMu.Unlock()
	if l.closed {
		return net.ErrClosed
	}
	select {
	case l.ch <- conn:
		return nil
	case <-time.After(time.Second):
		return errors.New("https listener queue is full")
	}
}

func (m *sniPassthroughMux) matchRoute(serverName string) (*sniPassthroughTarget, bool) {
	serverName = normalizeSNIName(serverName)
	if serverName == "" {
		return nil, false
	}
	if alias, _, ok := strings.Cut(serverName, "."); ok {
		if target, found := m.routes.Load(alias); found {
			return target, true
		}
	}
	return m.routes.Load(serverName)
}

func normalizeSNIName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name, _, _ = strings.Cut(name, ":")
	return name
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
	tee := io.TeeReader(conn, &captured)
	readConn := &clientHelloReadConn{Conn: conn, reader: tee}
	var serverName string
	tlsConn := tls.Server(readConn, &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			serverName = strings.ToLower(strings.TrimSpace(hello.ServerName))
			return nil, errClientHelloCaptured
		},
	})
	err := tlsConn.Handshake()
	replayed := &replayConn{
		Conn:   conn,
		reader: io.MultiReader(bytes.NewReader(captured.Bytes()), conn),
	}
	if errors.Is(err, errClientHelloCaptured) {
		return serverName, replayed, nil
	}
	return serverName, replayed, err
}
