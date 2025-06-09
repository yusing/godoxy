package stream

import (
	"context"
	"net"

	"github.com/rs/zerolog"
	nettypes "github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/utils"
	"go.uber.org/atomic"
)

type TCPTCPStream struct {
	listener *net.TCPListener
	laddr    *net.TCPAddr
	dst      *net.TCPAddr

	preDial nettypes.HookFunc
	onRead  nettypes.HookFunc

	closed atomic.Bool
}

func NewTCPTCPStream(listenAddr, dstAddr string) (nettypes.Stream, error) {
	dst, err := net.ResolveTCPAddr("tcp", dstAddr)
	if err != nil {
		return nil, err
	}
	laddr, err := net.ResolveTCPAddr("tcp", listenAddr)
	if err != nil {
		return nil, err
	}
	return &TCPTCPStream{laddr: laddr, dst: dst}, nil
}

func (s *TCPTCPStream) ListenAndServe(ctx context.Context, preDial, onRead nettypes.HookFunc) {
	listener, err := net.ListenTCP("tcp", s.laddr)
	if err != nil {
		logErr(s, err, "failed to listen")
		return
	}
	s.listener = listener
	s.preDial = preDial
	s.onRead = onRead
	go s.listen(ctx)
}

func (s *TCPTCPStream) Close() error {
	if s.closed.Swap(true) || s.listener == nil {
		return nil
	}
	return s.listener.Close()
}

func (s *TCPTCPStream) LocalAddr() net.Addr {
	if s.listener == nil {
		return s.laddr
	}
	return s.listener.Addr()
}

func (s *TCPTCPStream) MarshalZerologObject(e *zerolog.Event) {
	e.Str("protocol", "tcp-tcp").Str("listen", s.listener.Addr().String()).Str("dst", s.dst.String())
}

func (s *TCPTCPStream) listen(ctx context.Context) {
	for {
		if s.closed.Load() {
			return
		}

		select {
		case <-ctx.Done():
			return
		default:
			conn, err := s.listener.Accept()
			if err != nil {
				if s.closed.Load() {
					return
				}
				logErr(s, err, "failed to accept connection")
				continue
			}
			if s.onRead != nil {
				if err := s.onRead(ctx); err != nil {
					logErr(s, err, "failed to on read")
					continue
				}
			}
			go s.handle(ctx, conn)
		}
	}
}

func (s *TCPTCPStream) handle(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	if s.preDial != nil {
		if err := s.preDial(ctx); err != nil {
			if !s.closed.Load() {
				logErr(s, err, "failed to pre-dial")
			}
			return
		}
	}

	if s.closed.Load() {
		return
	}

	dstConn, err := net.DialTCP("tcp", nil, s.dst)
	if err != nil {
		if !s.closed.Load() {
			logErr(s, err, "failed to dial destination")
		}
		return
	}
	defer dstConn.Close()

	if s.closed.Load() {
		return
	}

	src := conn
	dst := net.Conn(dstConn)
	if s.onRead != nil {
		src = &wrapperConn{
			Conn:   conn,
			ctx:    ctx,
			onRead: s.onRead,
		}
		dst = &wrapperConn{
			Conn:   dstConn,
			ctx:    ctx,
			onRead: s.onRead,
		}
	}

	pipe := utils.NewBidirectionalPipe(ctx, src, dst)
	if err := pipe.Start(); err != nil && !s.closed.Load() {
		logErr(s, err, "error in bidirectional pipe")
	}
}

type wrapperConn struct {
	net.Conn
	ctx    context.Context
	onRead nettypes.HookFunc
}

func (w *wrapperConn) Read(b []byte) (n int, err error) {
	n, err = w.Conn.Read(b)
	if err != nil {
		return
	}
	if w.onRead != nil {
		if err = w.onRead(w.ctx); err != nil {
			return
		}
	}
	return
}
