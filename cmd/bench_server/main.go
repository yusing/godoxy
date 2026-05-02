package main

import (
	"flag"
	"log"
	"net/http"
	"time"
)

func main() {
	var (
		addr      = flag.String("addr", ":80", "TCP listen address")
		probe     = flag.String("probe", "", "probe type: http, sse, ws")
		probeURL  = flag.String("url", "", "probe URL")
		dialAddr  = flag.String("dial-addr", "", "dial target in host:port form")
		proto     = flag.String("proto", "h2", "protocol for HTTP/SSE probes: h1, h2, h3")
		method    = flag.String("method", http.MethodGet, "HTTP method for http probe")
		bodyBytes = flag.Int("body-bytes", 0, "request body size for HTTP probes")
		samples   = flag.Int("samples", 1, "number of probe samples to collect")
		timeout   = flag.Duration("timeout", 10*time.Second, "probe timeout")
	)
	flag.Parse()

	if *samples <= 0 {
		flag.Usage()
		log.Fatal("-samples must be > 0")
	}
	if *bodyBytes < 0 {
		flag.Usage()
		log.Fatal("-body-bytes must be >= 0")
	}

	if *probe != "" {
		cfg := probeConfig{
			Kind:      *probe,
			URL:       *probeURL,
			DialAddr:  *dialAddr,
			Proto:     *proto,
			Method:    *method,
			BodyBytes: *bodyBytes,
			Samples:   *samples,
			Timeout:   *timeout,
		}
		if err := runProbe(cfg); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := serveTCP(*addr); err != nil {
		log.Fatalf("serve tcp: %v", err)
	}
}
