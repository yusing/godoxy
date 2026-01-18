package stream

import (
	"context"
	"net"

	"github.com/pires/go-proxyproto"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/acl"
	"github.com/yusing/godoxy/internal/agentpool"
	"github.com/yusing/godoxy/internal/entrypoint"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	ioutils "github.com/yusing/goutils/io"
	"go.uber.org/atomic"
)

type TCPTCPStream struct {
	listener net.Listener

	network    string
	dstNetwork string

	laddr *net.TCPAddr
	dst   *net.TCPAddr
	agent *agentpool.Agent

	preDial nettypes.HookFunc
	onRead  nettypes.HookFunc

	closed atomic.Bool
}

func NewTCPTCPStream(network, dstNetwork, listenAddr, dstAddr string, agent *agentpool.Agent) (nettypes.Stream, error) {
	dst, err := net.ResolveTCPAddr(dstNetwork, dstAddr)
	if err != nil {
		return nil, err
	}
	laddr, err := net.ResolveTCPAddr(network, listenAddr)
	if err != nil {
		return nil, err
	}
	return &TCPTCPStream{network: network, dstNetwork: dstNetwork, laddr: laddr, dst: dst, agent: agent}, nil
}

func (s *TCPTCPStream) ListenAndServe(ctx context.Context, preDial, onRead nettypes.HookFunc) {
	var err error
	s.listener, err = net.ListenTCP(s.network, s.laddr)
	if err != nil {
		logErr(s, err, "failed to listen")
		return
	}

	if acl, ok := ctx.Value(acl.ContextKey{}).(*acl.Config); ok {
		log.Debug().Str("listener", s.listener.Addr().String()).Msg("wrapping listener with ACL")
		s.listener = acl.WrapTCP(s.listener)
	}

	if proxyProto := entrypoint.ActiveConfig.Load().SupportProxyProtocol; proxyProto {
		s.listener = &proxyproto.Listener{Listener: s.listener}
	}

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
	e.Str("protocol", s.network+"->"+s.dstNetwork)

	if s.listener != nil {
		e.Str("listen", s.listener.Addr().String())
	}
	if s.dst != nil {
		e.Str("dst", s.dst.String())
	}
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

	var (
		dstConn net.Conn
		err     error
	)
	if s.agent != nil {
		dstConn, err = s.agent.NewTCPClient(s.dst.String())
	} else {
		dstConn, err = net.DialTCP(s.dstNetwork, nil, s.dst)
	}
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
	dst := dstConn
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

	pipe := ioutils.NewBidirectionalPipe(ctx, src, dst)
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
		return n, err
	}
	if w.onRead != nil {
		if err = w.onRead(w.ctx); err != nil {
			return n, err
		}
	}
	return n, err
}
