package stream

import (
	"net"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/yusing/goutils/synk"
)

const (
	dialTimeout  = 10 * time.Second
	readDeadline = 10 * time.Second
)

// StreamALPN is the TLS ALPN protocol id used to multiplex the TCP stream tunnel
// and the HTTPS API on the same TCP port.
//
// When a client negotiates this ALPN, the agent will route the connection to the
// stream tunnel handler instead of the HTTP handler.
const StreamALPN = "godoxy-agent-stream/1"

var sizedPool = synk.GetSizedBytesPool()

type CreateConnFunc[Conn net.Conn] func(host, port string) (Conn, error)
type ConnectionManager[Conn net.Conn] struct {
	m                *xsync.Map[string, Conn]
	createConnection CreateConnFunc[Conn]
}

func NewConnectionManager[Conn net.Conn](createConnection CreateConnFunc[Conn]) *ConnectionManager[Conn] {
	return &ConnectionManager[Conn]{
		m:                xsync.NewMap[string, Conn](),
		createConnection: createConnection,
	}
}

func (c *ConnectionManager[Conn]) GetOrCreateDestConnection(clientConn net.Conn, host, port string) (ret Conn, connErr error) {
	clientKey := clientConn.RemoteAddr().String()
	ret, _ = c.m.LoadOrCompute(clientKey, func() (conn Conn, cancel bool) {
		conn, connErr = c.createConnection(host, port)
		if connErr != nil {
			cancel = true
		}
		return
	})

	return
}

func (c *ConnectionManager[Conn]) DeleteDestConnection(clientConn net.Conn) {
	clientKey := clientConn.RemoteAddr().String()
	conn, loaded := c.m.LoadAndDelete(clientKey)
	if loaded {
		conn.Close()
	}
}

func (c *ConnectionManager[Conn]) CloseAllConnections() {
	for _, conn := range c.m.Range {
		conn.Close()
	}
	c.m.Clear()
}
