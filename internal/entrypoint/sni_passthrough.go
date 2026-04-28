package entrypoint

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pires/go-proxyproto"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog/log"
	acl "github.com/yusing/godoxy/internal/acl/types"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/types"
)

var errClientHelloCaptured = errors.New("client hello captured")

const defaultClientHelloTimeout = 5 * time.Second

var clientHelloTimeout = defaultClientHelloTimeout

type sniPassthroughRoute interface {
	types.StreamRoute
	SNIPassthroughHosts() []string
}

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

func asSNIPassthroughRoute(route types.StreamRoute) (sniPassthroughRoute, bool) {
	routeWithSNI, ok := route.(sniPassthroughRoute)
	if !ok || len(routeWithSNI.SNIPassthroughHosts()) == 0 {
		return nil, false
	}
	return routeWithSNI, true
}

func (m *sniPassthroughManager) HTTPSListener(addr string) (net.Listener, bool, error) {
	mux, err := m.getOrStart(newSNIListenKeyForAddr(addr))
	if err != nil {
		return nil, false, err
	}
	return mux.httpsListener(), true, nil
}

func (m *sniPassthroughManager) AddRoute(route sniPassthroughRoute) error {
	mux, err := m.getOrStart(newSNIListenKey(route.ListenURL().Scheme, route.ListenURL().Host))
	if err != nil {
		return err
	}
	proxy, ok := route.Stream().(nettypes.ConnProxy)
	if !ok {
		return fmt.Errorf("route %q stream does not support accepted connection proxying", route.Name())
	}
	return mux.addRoute(route.Key(), route.SNIPassthroughHosts(), proxy, route.Task().Context())
}

func (m *sniPassthroughManager) DelRoute(route sniPassthroughRoute) {
	if mux, ok := m.muxes.Load(newSNIListenKey(route.ListenURL().Scheme, route.ListenURL().Host).String()); ok {
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
	ep       *Entrypoint
	key      sniListenKey
	base     net.Listener
	httpsMu  sync.Mutex
	https    *queuedListener
	routesMu sync.Mutex
	routes   *xsync.Map[string, *sniPassthroughTarget]
	matcher  atomic.Pointer[sniPatternMatcher]
	closed   atomic.Bool
}

type sniPassthroughTarget struct {
	key   string
	hosts []string
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
	mux.matcher.Store(newSNIPatternMatcher())
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

func (m *sniPassthroughMux) addRoute(key string, hosts []string, proxy nettypes.ConnProxy, ctx context.Context) error {
	if len(hosts) == 0 {
		return errors.New("sni passthrough route requires at least one host pattern")
	}
	target := &sniPassthroughTarget{key: key, hosts: slices.Clone(hosts), proxy: proxy, ctx: ctx}
	m.routesMu.Lock()
	defer m.routesMu.Unlock()
	m.routes.Store(key, target)
	m.rebuildMatcherLocked()
	return nil
}

func (m *sniPassthroughMux) delRoute(key string) {
	m.routesMu.Lock()
	defer m.routesMu.Unlock()
	m.routes.Delete(key)
	m.rebuildMatcherLocked()
}

func (m *sniPassthroughMux) rebuildMatcherLocked() {
	matcher := newSNIPatternMatcher()
	for _, target := range m.routes.Range {
		matcher.add(target.key, target.hosts)
	}
	m.matcher.Store(matcher)
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
	matcher := m.matcher.Load()
	if matcher != nil {
		if key, ok := matcher.match(serverName); ok {
			if target, found := m.routes.Load(key); found {
				target.proxy.ProxyConn(target.ctx, replayConn)
				return
			}
		}
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
	defer l.closedMu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	close(l.ch)
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

type sniPatternMatcher struct {
	exact     map[string]string
	wildcards []sniWildcardPattern
}

type sniWildcardPattern struct {
	suffix string
	key    string
}

func newSNIPatternMatcher() *sniPatternMatcher {
	return &sniPatternMatcher{exact: make(map[string]string)}
}

func (m *sniPatternMatcher) add(key string, hosts []string) {
	for _, host := range hosts {
		host = strings.ToLower(strings.TrimSpace(host))
		if strings.HasPrefix(host, "*.") {
			m.wildcards = append(m.wildcards, sniWildcardPattern{suffix: strings.TrimPrefix(host, "*"), key: key})
			continue
		}
		m.exact[host] = key
	}
	slices.SortFunc(m.wildcards, func(a, b sniWildcardPattern) int {
		return len(b.suffix) - len(a.suffix)
	})
}

func (m *sniPatternMatcher) match(serverName string) (string, bool) {
	serverName = strings.ToLower(strings.TrimSpace(serverName))
	if serverName == "" {
		return "", false
	}
	if key, ok := m.exact[serverName]; ok {
		return key, true
	}
	for _, wildcard := range m.wildcards {
		bareSuffix, _ := strings.CutPrefix(wildcard.suffix, ".")
		if serverName == bareSuffix {
			continue
		}
		if strings.HasSuffix(serverName, wildcard.suffix) && len(serverName) > len(wildcard.suffix) {
			return wildcard.key, true
		}
	}
	return "", false
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
