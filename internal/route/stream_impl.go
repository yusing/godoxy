package route

import (
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	gpnet "github.com/yusing/go-proxy/internal/net/types"
	U "github.com/yusing/go-proxy/internal/utils"
)

type (
	Stream struct {
		*StreamRoute

		listener   gpnet.StreamListener
		targetAddr net.Addr
	}
)

const (
	streamFirstConnBufferSize = 128
	streamDialTimeout         = 5 * time.Second
)

func NewStream(base *StreamRoute) *Stream {
	return &Stream{
		StreamRoute: base,
	}
}

func (stream *Stream) Addr() net.Addr {
	if stream.listener == nil {
		panic("listener is nil")
	}
	return stream.listener.Addr()
}

func (stream *Stream) Setup() error {
	var lcfg net.ListenConfig
	var err error

	ctx := stream.task.Context()

	switch stream.Scheme {
	case "tcp":
		stream.targetAddr, err = net.ResolveTCPAddr("tcp", stream.ProxyURL.Host)
		if err != nil {
			return err
		}
		tcpListener, err := lcfg.Listen(ctx, "tcp", stream.LisURL.Host)
		if err != nil {
			return err
		}
		// in case ListeningPort was zero, get the actual port
		stream.Port.Listening = tcpListener.Addr().(*net.TCPAddr).Port
		stream.listener = gpnet.NetListener(tcpListener)
	case "udp":
		stream.targetAddr, err = net.ResolveUDPAddr("udp", stream.ProxyURL.Host)
		if err != nil {
			return err
		}
		udpListener, err := lcfg.ListenPacket(ctx, "udp", stream.LisURL.Host)
		if err != nil {
			return err
		}
		udpConn, ok := udpListener.(*net.UDPConn)
		if !ok {
			udpListener.Close()
			return errors.New("udp listener is not *net.UDPConn")
		}
		stream.Port.Listening = udpConn.LocalAddr().(*net.UDPAddr).Port
		stream.listener = NewUDPForwarder(ctx, udpConn, stream.targetAddr)
	default:
		panic("should not reach here")
	}

	return nil
}

func (stream *Stream) Accept() (conn gpnet.StreamConn, err error) {
	if stream.listener == nil {
		return nil, errors.New("listener is nil")
	}
	// prevent Accept from blocking forever
	done := make(chan struct{})
	go func() {
		conn, err = stream.listener.Accept()
		close(done)
	}()

	select {
	case <-stream.task.Context().Done():
		stream.Close()
		return nil, stream.task.Context().Err()
	case <-done:
		return conn, nil
	}
}

func (stream *Stream) Handle(conn gpnet.StreamConn) error {
	switch conn := conn.(type) {
	case *UDPConn:
		switch stream := stream.listener.(type) {
		case *UDPForwarder:
			return stream.Handle(conn)
		default:
			return fmt.Errorf("unexpected listener type: %T", stream)
		}
	case io.ReadWriteCloser:
		dialer := &net.Dialer{Timeout: streamDialTimeout}
		dstConn, err := dialer.DialContext(stream.task.Context(), stream.targetAddr.Network(), stream.targetAddr.String())
		if err != nil {
			return err
		}
		defer dstConn.Close()
		defer conn.Close()
		pipe := U.NewBidirectionalPipe(stream.task.Context(), conn, dstConn)
		return pipe.Start()
	default:
		return fmt.Errorf("unexpected conn type: %T", conn)
	}
}

func (stream *Stream) Close() error {
	return stream.listener.Close()
}
