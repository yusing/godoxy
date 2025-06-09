package stream

import (
	"bytes"
	"context"
	"maps"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog"
	nettypes "github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/utils/synk"
	"go.uber.org/atomic"
)

type UDPUDPStream struct {
	name     string
	listener *net.UDPConn

	laddr *net.UDPAddr
	dst   *net.UDPAddr

	preDial nettypes.HookFunc
	onRead  nettypes.HookFunc

	cleanUpTicker *time.Ticker

	conns  map[string]*udpUDPConn
	closed atomic.Bool
	mu     sync.Mutex
}

type udpUDPConn struct {
	srcAddr  *net.UDPAddr
	dstConn  *net.UDPConn
	listener *net.UDPConn
	lastUsed atomic.Time
	closed   atomic.Bool
	mu       sync.Mutex
}

const (
	udpBufferSize      = 16 * 1024
	udpIdleTimeout     = 5 * time.Minute // Longer timeout for game sessions
	udpCleanupInterval = 1 * time.Minute
	udpReadTimeout     = 30 * time.Second
)

var bufPool = synk.NewBytesPool()

func NewUDPUDPStream(listenAddr, dstAddr string) (nettypes.Stream, error) {
	dst, err := net.ResolveUDPAddr("udp", dstAddr)
	if err != nil {
		return nil, err
	}
	laddr, err := net.ResolveUDPAddr("udp", listenAddr)
	if err != nil {
		return nil, err
	}
	return &UDPUDPStream{
		laddr: laddr,
		dst:   dst,
		conns: make(map[string]*udpUDPConn),
	}, nil
}

func (s *UDPUDPStream) ListenAndServe(ctx context.Context, preDial, onRead nettypes.HookFunc) {
	listener, err := net.ListenUDP("udp", s.laddr)
	if err != nil {
		logErr(s, err, "failed to listen")
		return
	}
	s.listener = listener
	s.preDial = preDial
	s.onRead = onRead
	go s.listen(ctx)
	go s.cleanUp(ctx)
}

func (s *UDPUDPStream) Close() error {
	if s.closed.Swap(true) || s.listener == nil {
		return nil
	}

	var wg sync.WaitGroup
	s.mu.Lock()
	for _, conn := range s.conns {
		wg.Add(1)
		go func(c *udpUDPConn) {
			defer wg.Done()
			c.Close()
		}(conn)
	}
	clear(s.conns)
	s.mu.Unlock()

	wg.Wait()

	return s.listener.Close()
}

func (s *UDPUDPStream) LocalAddr() net.Addr {
	if s.listener == nil {
		return s.laddr
	}
	return s.listener.LocalAddr()
}

func (s *UDPUDPStream) MarshalZerologObject(e *zerolog.Event) {
	e.Str("protocol", "udp-udp").Str("name", s.name).Str("dst", s.dst.String())
}

func (s *UDPUDPStream) listen(ctx context.Context) {
	buf := bufPool.GetSized(udpBufferSize)
	defer bufPool.Put(buf)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			n, srcAddr, err := s.listener.ReadFromUDP(buf)
			if err != nil {
				if s.closed.Load() {
					return
				}
				logErr(s, err, "failed to read from listener")
				continue
			}

			logDebugf(s, "read %d bytes from %s", n, srcAddr)

			if s.onRead != nil {
				if err := s.onRead(ctx); err != nil {
					logErr(s, err, "failed to on read")
					continue
				}
			}

			// Get or create connection, passing the initial data
			go s.getOrCreateConnection(ctx, srcAddr, bytes.Clone(buf[:n]))
		}
	}
}

func (s *UDPUDPStream) getOrCreateConnection(ctx context.Context, srcAddr *net.UDPAddr, initialData []byte) {
	key := srcAddr.String()

	s.mu.Lock()
	if conn, ok := s.conns[key]; ok {
		s.mu.Unlock()
		// Forward packet for existing connection
		go conn.forwardToDestination(initialData)
		return
	}

	defer s.mu.Unlock()
	// Create new connection with initial data
	conn, ok := s.createConnection(ctx, srcAddr, initialData)
	if ok && !conn.closed.Load() {
		s.conns[key] = conn
	}
}

func (s *UDPUDPStream) createConnection(ctx context.Context, srcAddr *net.UDPAddr, initialData []byte) (*udpUDPConn, bool) {
	// Apply pre-dial if configured
	if s.preDial != nil {
		if err := s.preDial(ctx); err != nil {
			logErr(s, err, "failed to pre-dial")
			return nil, false
		}
	}

	// Create UDP connection to destination
	dstConn, err := net.DialUDP("udp", nil, s.dst)
	if err != nil {
		logErr(s, err, "failed to dial dst")
		return nil, false
	}

	conn := &udpUDPConn{
		srcAddr:  srcAddr,
		dstConn:  dstConn,
		listener: s.listener,
	}
	conn.lastUsed.Store(time.Now())

	// Send initial data before starting response handler
	if !conn.forwardToDestination(initialData) {
		dstConn.Close()
		return nil, false
	}

	// Start response handler after initial data is sent
	go conn.handleResponses(ctx)

	logDebugf(s, "created new connection from %s", srcAddr.String())
	return conn, true
}

func (conn *udpUDPConn) MarshalZerologObject(e *zerolog.Event) {
	e.Stringer("src", conn.srcAddr).Stringer("dst", conn.dstConn.RemoteAddr())
}

func (conn *udpUDPConn) handleResponses(ctx context.Context) {
	buf := bufPool.GetSized(udpBufferSize)
	defer bufPool.Put(buf)

	defer conn.Close()

	for {
		if conn.closed.Load() {
			return
		}

		select {
		case <-ctx.Done():
			return
		default:
			// Set a reasonable timeout for reads
			_ = conn.dstConn.SetReadDeadline(time.Now().Add(udpReadTimeout))

			n, err := conn.dstConn.Read(buf)
			if err != nil {
				if !conn.closed.Load() {
					logErr(conn, err, "failed to read from dst")
				}
				return
			}

			// Clear deadline after successful read
			_ = conn.dstConn.SetReadDeadline(time.Time{})

			// Forward response back to client using the listener
			_, err = conn.listener.WriteToUDP(buf[:n], conn.srcAddr)
			if err != nil {
				if !conn.closed.Load() {
					logErrf(conn, err, "failed to write %d bytes to client", n)
				}
				return
			}

			conn.lastUsed.Store(time.Now())
			logDebugf(conn, "forwarded response to client, %d bytes", n)
		}
	}
}

func (s *UDPUDPStream) cleanUp(ctx context.Context) {
	s.cleanUpTicker = time.NewTicker(udpCleanupInterval)
	defer s.cleanUpTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.cleanUpTicker.C:
			s.mu.Lock()
			conns := maps.Clone(s.conns)
			s.mu.Unlock()

			removed := []string(nil)
			for key, conn := range conns {
				if conn.Expired() {
					conn.Close()
					removed = append(removed, key)
				}
			}

			s.mu.Lock()
			for _, key := range removed {
				logDebugf(s, "cleaning up expired connection: %s", key)
				delete(s.conns, key)
			}
			s.mu.Unlock()
		}
	}
}

func (conn *udpUDPConn) forwardToDestination(data []byte) bool {
	conn.mu.Lock()
	defer conn.mu.Unlock()

	if conn.closed.Load() {
		return false
	}

	_, err := conn.dstConn.Write(data)
	if err != nil {
		logErrf(conn, err, "failed to write %d bytes to dst", len(data))
		return false
	}

	conn.lastUsed.Store(time.Now())
	logDebugf(conn, "forwarded %d bytes to dst", len(data))
	return true
}

func (conn *udpUDPConn) Expired() bool {
	return time.Since(conn.lastUsed.Load()) > udpIdleTimeout
}

func (conn *udpUDPConn) Close() {
	conn.mu.Lock()
	defer conn.mu.Unlock()

	if conn.closed.Load() {
		return
	}

	conn.closed.Store(true)

	conn.dstConn.Close()
	conn.dstConn = nil
}
