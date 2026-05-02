package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"
)

func main() {
	var (
		addr      = flag.String("addr", ":80", "HTTP listen address")
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

	handler := newBenchHandler()
	server := &http.Server{
		Addr:    *addr,
		Handler: handler,
	}

	log.Printf("Bench server listening on %s", *addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("ListenAndServe: %v", err)
	}
}

func unsupportedResponseWriter(w http.ResponseWriter, feature string) {
	http.Error(w, fmt.Sprintf("%s is not supported by this response writer", feature), http.StatusInternalServerError)
}
