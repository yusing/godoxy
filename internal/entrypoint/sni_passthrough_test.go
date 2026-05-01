package entrypoint

import (
	"crypto/tls"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/common"
	netutils "github.com/yusing/godoxy/internal/net"
)

func TestSNIMatchUsesAliasOrFQDN(t *testing.T) {
	routes := map[string]bool{
		"site2":             true,
		"fqdn.example.test": true,
	}
	exists := func(key string) bool { return routes[key] }

	key, ok := matchSNI("site2.example.test", findRouteKeyAnyDomain, exists)
	require.True(t, ok)
	require.Equal(t, "site2", key)

	key, ok = matchSNI("fqdn.example.test", findRouteKeyAnyDomain, exists)
	require.True(t, ok)
	require.Equal(t, "fqdn.example.test", key)

	_, ok = matchSNI("other.example.test", findRouteKeyAnyDomain, exists)
	require.False(t, ok)
}

func TestSNIMatchHonorsDomainFilters(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)
	ep.SetFindRouteDomains([]string{".example.test"})
	exists := func(key string) bool { return key == "db" }

	key, ok := matchSNI("db.example.test", ep.findRouteKeyFunc, exists)
	require.True(t, ok)
	require.Equal(t, "db", key)

	_, ok = matchSNI("db.attacker.test", ep.findRouteKeyFunc, exists)
	require.False(t, ok)
}

func TestSNIRouterListensPerHTTPSAddress(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)
	t.Cleanup(func() {
		require.NoError(t, ep.sni.Close())
	})

	addr1 := reserveSNIListenAddr(t)
	addr2 := reserveSNIListenAddr(t)

	listener1, err := ep.sni.Listen(addr1)
	require.NoError(t, err)
	listener2, err := ep.sni.Listen(addr2)
	require.NoError(t, err)

	require.NotSame(t, listener1, listener2)
	require.Equal(t, addr1, listener1.Addr().String())
	require.Equal(t, addr2, listener2.Addr().String())
	require.Equal(t, 2, ep.sni.listeners.Size())

	again, err := ep.sni.Listen(addr1)
	require.NoError(t, err)
	require.Same(t, listener1, again)
}

func TestSharedHTTPSListenAddrMatchesConfiguredWildcard(t *testing.T) {
	port := strconv.Itoa(common.ProxyHTTPSPort)
	require.True(t, netutils.IsSharedHTTPSListenAddr(common.ProxyHTTPSAddr))

	isConfiguredWildcard := netutils.IsWildcardListenHost(common.ProxyHTTPSAddr)
	for _, addr := range []string{
		net.JoinHostPort("0.0.0.0", port),
		net.JoinHostPort("::", port),
		net.JoinHostPort("0:0:0:0:0:0:0:0", port),
	} {
		a := addr
		t.Run(a, func(t *testing.T) {
			require.Equal(t, isConfiguredWildcard, netutils.IsSharedHTTPSListenAddr(a), a)
			require.True(t, netutils.IsWildcardListenHost(a), a)
		})
	}

	otherPort := "1"
	if port == otherPort {
		otherPort = "2"
	}
	require.False(t, netutils.IsSharedHTTPSListenAddr(net.JoinHostPort("0.0.0.0", otherPort)))
	require.False(t, netutils.IsWildcardListenHost(net.JoinHostPort("127.0.0.1", port)))
}

func TestReadClientHelloServerNameReplaysBytes(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	clientErr := make(chan error, 1)
	go func() {
		client := tls.Client(clientConn, &tls.Config{
			ServerName:         "WWW.Site2.Domain.TLD",
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS12,
		})
		clientErr <- client.Handshake()
	}()

	serverName, replayConn, err := readClientHelloServerName(serverConn)
	require.NoError(t, err)
	require.Equal(t, "www.site2.domain.tld", serverName)

	_ = replayConn.SetReadDeadline(time.Now().Add(time.Second))
	first := make([]byte, 1)
	_, err = io.ReadFull(replayConn, first)
	require.NoError(t, err)
	require.Equal(t, byte(0x16), first[0])

	clientConn.Close()
	require.Error(t, <-clientErr)
}

func reserveSNIListenAddr(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())
	return addr
}
