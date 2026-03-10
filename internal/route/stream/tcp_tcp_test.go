package stream

import (
	"bufio"
	"context"
	"io"
	"net"
	"testing"

	"github.com/pires/go-proxyproto"
	entrypoint "github.com/yusing/godoxy/internal/entrypoint"
	entrypointtypes "github.com/yusing/godoxy/internal/entrypoint/types"
	"github.com/yusing/goutils/task"

	"github.com/stretchr/testify/require"
)

func TestTCPTCPStreamRelayProxyProtocolHeader(t *testing.T) {
	t.Run("Disabled", func(t *testing.T) {
		upstreamLn, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer upstreamLn.Close()

		s, err := NewTCPTCPStream("tcp", "tcp", "127.0.0.1:0", upstreamLn.Addr().String(), nil, false)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		require.NoError(t, s.ListenAndServe(ctx, nil, nil))
		defer s.Close()

		client, err := net.Dial("tcp", s.LocalAddr().String())
		require.NoError(t, err)
		defer client.Close()

		_, err = client.Write([]byte("ping"))
		require.NoError(t, err)

		upstreamConn, err := upstreamLn.Accept()
		require.NoError(t, err)
		defer upstreamConn.Close()

		payload := make([]byte, 4)
		_, err = io.ReadFull(upstreamConn, payload)
		require.NoError(t, err)
		require.Equal(t, []byte("ping"), payload)
	})

	t.Run("Enabled", func(t *testing.T) {
		upstreamLn, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer upstreamLn.Close()

		s, err := NewTCPTCPStream("tcp", "tcp", "127.0.0.1:0", upstreamLn.Addr().String(), nil, true)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		require.NoError(t, s.ListenAndServe(ctx, nil, nil))
		defer s.Close()

		client, err := net.Dial("tcp", s.LocalAddr().String())
		require.NoError(t, err)
		defer client.Close()

		_, err = client.Write([]byte("ping"))
		require.NoError(t, err)

		upstreamConn, err := upstreamLn.Accept()
		require.NoError(t, err)
		defer upstreamConn.Close()

		reader := bufio.NewReader(upstreamConn)
		header, err := proxyproto.Read(reader)
		require.NoError(t, err)
		require.Equal(t, proxyproto.PROXY, header.Command)

		srcAddr, ok := header.SourceAddr.(*net.TCPAddr)
		require.True(t, ok)
		dstAddr, ok := header.DestinationAddr.(*net.TCPAddr)
		require.True(t, ok)
		require.Equal(t, client.LocalAddr().String(), srcAddr.String())
		require.Equal(t, s.LocalAddr().String(), dstAddr.String())

		payload := make([]byte, 4)
		_, err = io.ReadFull(reader, payload)
		require.NoError(t, err)
		require.Equal(t, []byte("ping"), payload)
	})
}

func TestTCPTCPStreamRelayProxyProtocolUsesIncomingProxyHeader(t *testing.T) {
	upstreamLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer upstreamLn.Close()

	s, err := NewTCPTCPStream("tcp", "tcp", "127.0.0.1:0", upstreamLn.Addr().String(), nil, true)
	require.NoError(t, err)

	parent := task.GetTestTask(t)
	ep := entrypoint.NewEntrypoint(parent, &entrypoint.Config{
		SupportProxyProtocol: true,
	})
	entrypointtypes.SetCtx(parent, ep)

	ctx, cancel := context.WithCancel(parent.Context())
	defer cancel()
	require.NoError(t, s.ListenAndServe(ctx, nil, nil))
	defer s.Close()

	client, err := net.Dial("tcp", s.LocalAddr().String())
	require.NoError(t, err)
	defer client.Close()

	downstreamHeader := &proxyproto.Header{
		Version:           2,
		Command:           proxyproto.PROXY,
		TransportProtocol: proxyproto.TCPv4,
		SourceAddr: &net.TCPAddr{
			IP:   net.ParseIP("203.0.113.10"),
			Port: 42300,
		},
		DestinationAddr: &net.TCPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: s.LocalAddr().(*net.TCPAddr).Port,
		},
	}
	_, err = downstreamHeader.WriteTo(client)
	require.NoError(t, err)

	_, err = client.Write([]byte("pong"))
	require.NoError(t, err)

	upstreamConn, err := upstreamLn.Accept()
	require.NoError(t, err)
	defer upstreamConn.Close()

	reader := bufio.NewReader(upstreamConn)
	header, err := proxyproto.Read(reader)
	require.NoError(t, err)
	require.Equal(t, downstreamHeader.SourceAddr.String(), header.SourceAddr.String())
	require.Equal(t, downstreamHeader.DestinationAddr.String(), header.DestinationAddr.String())

	payload := make([]byte, 4)
	_, err = io.ReadFull(reader, payload)
	require.NoError(t, err)
	require.Equal(t, []byte("pong"), payload)
}
