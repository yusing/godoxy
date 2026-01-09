package stream

import (
	"time"

	"github.com/pion/dtls/v3"
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

var dTLSCipherSuites = []dtls.CipherSuiteID{dtls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256}

var sizedPool = synk.GetSizedBytesPool()
