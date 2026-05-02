package entrypoint

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pires/go-proxyproto"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog/log"
	acl "github.com/yusing/godoxy/internal/acl/types"
	autocert "github.com/yusing/godoxy/internal/autocert/types"
	netutils "github.com/yusing/godoxy/internal/net"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/types"
	"golang.org/x/sys/unix"
)

var errClientHelloRead = errors.New("client hello read")

const clientHelloTimeout = 5 * time.Second

type sniRouter struct {
	ep        *Entrypoint
	listeners *xsync.Map[string, *sniListener]
}

type sniListener struct {
	net.Listener
	router *sniRouter
	addr   string

	routes   atomic.Pointer[sniRouteTable]
	state    atomic.Uint32
	sniffing atomic.Bool

	queue sniConnQueue
}

type sniRouteTable struct {
	byKey map[string]*sniRouteEntry
}

func (t *sniRouteTable) exists(key string) bool {
	_, ok := t.byKey[normalizeSNIName(key)]
	return ok
}

type sniRouteEntry struct {
	route         types.StreamRoute
	proxy         nettypes.ConnProxy
	terminatesTLS bool
}

func newSNIRouter(ep *Entrypoint) *sniRouter {
	r := &sniRouter{
		ep:        ep,
		listeners: xsync.NewMap[string, *sniListener](),
	}
	ep.task.OnCancel("sni_router", func() { _ = r.Close() })
	return r
}

func (r *sniRouter) AddRoute(route types.StreamRoute) error {
	proxy, ok := route.Stream().(nettypes.ConnProxy)
	if !ok {
		return fmt.Errorf("route %q stream does not support accepted connection proxying", route.Name())
	}
	ctx := r.ep.task.Context()
	terminatesTLS := routeTerminatesTLS(route)
	if terminatesTLS && autocert.FromCtx(ctx) == nil {
		return fmt.Errorf("route %q tls_termination requires an autocert provider", route.Name())
	}
	addr := sniListenAddr(route)
	listener, err := r.Listen(ctx, addr)
	if err != nil {
		return err
	}
	sniListener := listener.(*sniListener)
	sniListener.addRoute(route, proxy, terminatesTLS)
	sniListener.ensureSniffing()
	return nil
}

func (r *sniRouter) DelRoute(route types.StreamRoute) {
	if listener, ok := r.listeners.Load(sniListenAddr(route)); ok {
		listener.delRoute(route)
	}
}

func (r *sniRouter) Listen(ctx context.Context, addr string) (net.Listener, error) {
	var listenErr error
	listener, loaded := r.listeners.LoadOrCompute(addr, func() (*sniListener, bool) {
		var lc net.ListenConfig
		ln, err := lc.Listen(ctx, "tcp", addr)
		if err != nil {
			listenErr = err
			return nil, true
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
		}
		listener.routes.Store(&sniRouteTable{byKey: map[string]*sniRouteEntry{}})
		listener.queue.init()
		return listener, false
	})
	if listenErr != nil {
		return nil, listenErr
	}
	if !loaded && listener == nil {
		return nil, net.ErrClosed
	}
	return listener, nil
}

func (r *sniRouter) Close() error {
	var err error
	for addr, listener := range r.listeners.AllRelaxed() {
		err = errors.Join(err, listener.Close())
		r.listeners.Delete(addr)
	}
	return err
}

func (r *sniRouter) accept(listener *sniListener) {
	for {
		conn, err := listener.Listener.Accept()
		if err != nil {
			current, ok := r.listeners.Load(listener.addr)
			if !ok || current != listener || errors.Is(err, net.ErrClosed) {
				return
			}
			log.Err(err).Msg("failed to accept sni-routed connection")
			continue
		}
		if listener.emptyRoutes() {
			r.forwardHTTPS(listener, conn)
			continue
		}
		go r.handle(listener, conn)
	}
}

func (r *sniRouter) handle(listener *sniListener, conn net.Conn) {
	if listener.emptyRoutes() {
		r.forwardHTTPS(listener, conn)
		return
	}

	_ = conn.SetReadDeadline(time.Now().Add(clientHelloTimeout))
	serverName, replayConn, err := readClientHelloServerName(conn)
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil {
		r.forwardHTTPS(listener, replayConn)
		return
	}
	entry, ok := listener.match(serverName, r.ep.findRouteKeyFunc)
	if !ok {
		r.forwardHTTPS(listener, replayConn)
		return
	}
	if entry.terminatesTLS {
		terminatedConn, err := r.terminateTLS(entry.route.Task().Context(), replayConn)
		if err != nil {
			log.Debug().Err(err).Str("server_name", serverName).Msg("failed to terminate tls for tcp route")
			_ = replayConn.Close()
			return
		}
		replayConn = terminatedConn
	}
	entry.proxy.ProxyConn(entry.route.Task().Context(), replayConn)
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

func (l *sniListener) addRoute(route types.StreamRoute, proxy nettypes.ConnProxy, terminatesTLS bool) {
	key := normalizeSNIName(route.Key())
	entry := &sniRouteEntry{route: route, proxy: proxy, terminatesTLS: terminatesTLS}
	for {
		old := l.routes.Load()
		if old == nil {
			old = &sniRouteTable{byKey: map[string]*sniRouteEntry{}}
		}
		next := &sniRouteTable{byKey: make(map[string]*sniRouteEntry, len(old.byKey)+1)}
		maps.Copy(next.byKey, old.byKey)
		next.byKey[key] = entry
		if l.routes.CompareAndSwap(old, next) {
			return
		}
		runtime.Gosched()
	}
}

func (l *sniListener) delRoute(route types.StreamRoute) {
	key := normalizeSNIName(route.Key())
	for {
		old := l.routes.Load()
		if old == nil || old.byKey[key] == nil {
			return
		}
		next := &sniRouteTable{byKey: make(map[string]*sniRouteEntry, len(old.byKey)-1)}
		for k, v := range old.byKey {
			if k != key {
				next.byKey[k] = v
			}
		}
		if l.routes.CompareAndSwap(old, next) {
			return
		}
		runtime.Gosched()
	}
}

func (l *sniListener) emptyRoutes() bool {
	table := l.routes.Load()
	return table == nil || len(table.byKey) == 0
}

func (l *sniListener) match(serverName string, findKey findRouteKeyFunc) (*sniRouteEntry, bool) {
	table := l.routes.Load()
	if table == nil || len(table.byKey) == 0 {
		return nil, false
	}
	key, ok := matchSNI(serverName, findKey, table.exists)
	if !ok {
		return nil, false
	}
	entry, ok := table.byKey[normalizeSNIName(key)]
	return entry, ok
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

func (l *sniListener) ensureSniffing() {
	// Keep the shared HTTPS listener on the queue-backed dispatch loop once it is
	// used so a late-added TCP SNI route cannot be bypassed by an older raw
	// Accept call that was already blocked on the underlying listener.
	if l.sniffing.CompareAndSwap(false, true) {
		go l.router.accept(l)
	}
}

func (l *sniListener) Accept() (net.Conn, error) {
	for {
		if !l.sniffing.Load() {
			conn, err := l.Listener.Accept()
			if err != nil {
				return nil, err
			}
			if l.sniffing.Load() {
				go l.router.handle(l, conn)
				continue
			}
			return conn, nil
		}
		conn, ok := l.queue.pop()
		if !ok {
			return nil, net.ErrClosed
		}
		return conn, nil
	}
}

func (l *sniListener) Close() error {
	if !l.state.CompareAndSwap(0, 1) {
		return nil
	}
	err := l.Listener.Close()
	l.router.listeners.Delete(l.addr)
	l.queue.close()
	return err
}

func (l *sniListener) forwardHTTPS(conn net.Conn) {
	if !l.queue.push(conn) {
		_ = conn.Close()
	}
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

type sniConnQueue struct {
	stub    sniConnQueueNode
	head    atomic.Pointer[sniConnQueueNode]
	tail    *sniConnQueueNode
	popMu   sync.Mutex
	waiting atomic.Bool
	closed  atomic.Bool
	pushers atomic.Int64
	notify  int
}

type sniConnQueueNode struct {
	next atomic.Pointer[sniConnQueueNode]
	conn net.Conn
}

var sniConnQueueNodePool = sync.Pool{
	New: func() any { return new(sniConnQueueNode) },
}

func acquireSNIConnQueueNode(conn net.Conn) *sniConnQueueNode {
	node := sniConnQueueNodePool.Get().(*sniConnQueueNode)
	node.next.Store(nil)
	node.conn = conn
	return node
}

func releaseSNIConnQueueNode(node *sniConnQueueNode) {
	if node == nil {
		return
	}
	node.next.Store(nil)
	node.conn = nil
	sniConnQueueNodePool.Put(node)
}

func (q *sniConnQueue) init() {
	q.notify = -1
	q.head.Store(&q.stub)
	q.tail = &q.stub
	fd, err := unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
	if err == nil {
		q.notify = fd
	}
}

func (q *sniConnQueue) push(conn net.Conn) bool {
	for {
		if q.closed.Load() {
			return false
		}
		q.pushers.Add(1)
		if !q.closed.Load() {
			break
		}
		q.pushers.Add(-1)
	}
	node := acquireSNIConnQueueNode(conn)
	prev := q.head.Swap(node)
	prev.next.Store(node)
	q.pushers.Add(-1)
	if q.waiting.Swap(false) {
		q.wake()
	}
	return true
}

func (q *sniConnQueue) pop() (net.Conn, bool) {
	q.popMu.Lock()
	defer q.popMu.Unlock()
	for {
		if conn, ok := q.tryPopLocked(); ok {
			q.waiting.Store(false)
			return conn, true
		}
		if q.closed.Load() {
			q.waiting.Store(false)
			return nil, false
		}
		q.waiting.Store(true)
		if conn, ok := q.tryPopLocked(); ok {
			q.waiting.Store(false)
			return conn, true
		}
		if q.closed.Load() {
			q.waiting.Store(false)
			return nil, false
		}
		q.wait()
	}
}

func (q *sniConnQueue) close() {
	if q.closed.Swap(true) {
		return
	}
	q.wake()
	for q.pushers.Load() != 0 {
		runtime.Gosched()
	}
	q.popMu.Lock()
	defer q.popMu.Unlock()
	for {
		conn, ok := q.tryPopLocked()
		if !ok {
			break
		}
		_ = conn.Close()
	}
	if q.notify >= 0 {
		_ = unix.Close(q.notify)
		q.notify = -1
	}
}

func (q *sniConnQueue) tryPopLocked() (net.Conn, bool) {
	tail := q.tail
	next := tail.next.Load()
	if next == nil {
		return nil, false
	}
	q.tail = next
	conn := next.conn
	next.conn = nil
	if tail != &q.stub {
		releaseSNIConnQueueNode(tail)
	}
	return conn, true
}

func (q *sniConnQueue) wake() {
	if q.notify < 0 {
		return
	}
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], 1)
	_, _ = unix.Write(q.notify, buf[:])
}

func (q *sniConnQueue) wait() {
	if q.notify < 0 {
		time.Sleep(50 * time.Microsecond)
		return
	}
	pollFds := []unix.PollFd{{Fd: int32(q.notify), Events: unix.POLLIN}}
	_, _ = unix.Poll(pollFds, -1)
	var buf [8]byte
	for {
		_, err := unix.Read(q.notify, buf[:])
		if err != nil {
			return
		}
	}
}
