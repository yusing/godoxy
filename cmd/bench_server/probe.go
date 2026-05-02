package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

type probeConfig struct {
	Kind      string
	URL       string
	DialAddr  string
	Proto     string
	Method    string
	BodyBytes int
	Samples   int
	Timeout   time.Duration
}

type probeSample struct {
	Dial    time.Duration
	TTFB    time.Duration
	Total   time.Duration
	Status  int
	Bytes   int64
	ErrText string
}

func runProbe(cfg probeConfig) error {
	if cfg.URL == "" {
		return errors.New("-url is required in probe mode")
	}
	if cfg.Samples <= 0 {
		cfg.Samples = 1
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	cfg.Kind = strings.ToLower(cfg.Kind)
	cfg.Proto = strings.ToLower(cfg.Proto)
	cfg.Method = strings.ToUpper(cfg.Method)
	if cfg.Proto == "" {
		cfg.Proto = "h2"
	}

	fmt.Printf("Probe kind=%s proto=%s url=%s samples=%d\n", cfg.Kind, cfg.Proto, cfg.URL, cfg.Samples)
	if cfg.DialAddr != "" {
		fmt.Printf("Dial target override=%s\n", cfg.DialAddr)
	}

	samples := make([]probeSample, 0, cfg.Samples)
	for i := 0; i < cfg.Samples; i++ {
		sample, err := runProbeSample(cfg)
		if err != nil {
			sample.ErrText = err.Error()
		}
		samples = append(samples, sample)
		fmt.Printf("  sample=%d dial_ms=%.3f ttfb_ms=%.3f total_ms=%.3f status=%d bytes=%d",
			i+1,
			durationMillis(sample.Dial),
			durationMillis(sample.TTFB),
			durationMillis(sample.Total),
			sample.Status,
			sample.Bytes,
		)
		if sample.ErrText != "" {
			fmt.Printf(" error=%q", sample.ErrText)
		}
		fmt.Println()
	}

	failed := printProbeSummary(samples)
	if failed > 0 {
		return fmt.Errorf("%d/%d probe samples failed", failed, len(samples))
	}
	return nil
}

func runProbeSample(cfg probeConfig) (probeSample, error) {
	switch cfg.Kind {
	case "http":
		return probeHTTP(cfg)
	case "sse":
		return probeSSE(cfg)
	case "ws":
		return probeWS(cfg)
	default:
		return probeSample{}, fmt.Errorf("unsupported probe kind %q", cfg.Kind)
	}
}

func probeHTTP(cfg probeConfig) (probeSample, error) {
	return probeHTTPFamily(cfg, false)
}

func probeSSE(cfg probeConfig) (probeSample, error) {
	return probeHTTPFamily(cfg, true)
}

func probeHTTPFamily(cfg probeConfig, readFirstEvent bool) (probeSample, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	transport, dialTracker, cleanup, err := newRoundTripper(cfg)
	if err != nil {
		return probeSample{}, err
	}
	defer cleanup()

	client := &http.Client{Transport: transport}
	body := requestBody(cfg.BodyBytes)
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, cfg.Method, cfg.URL, reqBody)
	if err != nil {
		return probeSample{}, err
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/octet-stream")
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return probeSample{Dial: dialTracker.Duration()}, err
	}
	defer resp.Body.Close()

	sample := probeSample{Dial: dialTracker.Duration(), Status: resp.StatusCode}

	var n int64
	if readFirstEvent {
		n, err = readFirstSSEEvent(resp.Body)
	} else {
		n, err = readFirstByte(resp.Body)
	}
	sample.TTFB = time.Since(start)
	sample.Bytes += n
	if err != nil && !errors.Is(err, io.EOF) {
		sample.Total = time.Since(start)
		return sample, err
	}

	rest, err := io.Copy(io.Discard, resp.Body)
	sample.Bytes += rest
	sample.Total = time.Since(start)
	if err != nil {
		return sample, err
	}
	return sample, nil
}

func probeWS(cfg probeConfig) (probeSample, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	u, err := url.Parse(cfg.URL)
	if err != nil {
		return probeSample{}, err
	}

	dialer := websocket.Dialer{
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: true, ServerName: u.Hostname()}, //nolint:gosec // local benchmark cert
		HandshakeTimeout: cfg.Timeout,
	}
	if cfg.DialAddr != "" {
		netDialer := &net.Dialer{Timeout: cfg.Timeout}
		dialer.NetDialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
			return netDialer.DialContext(ctx, network, cfg.DialAddr)
		}
	}

	start := time.Now()
	conn, resp, err := dialer.DialContext(ctx, cfg.URL, http.Header{})
	dialDuration := time.Since(start)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		return probeSample{Dial: dialDuration, Status: status}, err
	}
	defer conn.Close()

	sample := probeSample{Dial: dialDuration, Status: http.StatusSwitchingProtocols}
	_, payload, err := conn.ReadMessage()
	sample.TTFB = time.Since(start)
	sample.Total = sample.TTFB
	sample.Bytes = int64(len(payload))
	if err != nil {
		return sample, err
	}

	closeMessage := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "benchmark complete")
	_ = conn.WriteControl(websocket.CloseMessage, closeMessage, time.Now().Add(time.Second))
	return sample, nil
}

type dialDurationRecorder struct {
	start time.Time
	end   time.Time
	set   bool
	value time.Duration
}

func (d *dialDurationRecorder) Begin() {
	d.start = time.Now()
}

func (d *dialDurationRecorder) End() {
	d.end = time.Now()
	d.value = d.end.Sub(d.start)
	d.set = true
}

func (d *dialDurationRecorder) Duration() time.Duration {
	if d.set {
		return d.value
	}
	return 0
}

func newRoundTripper(cfg probeConfig) (http.RoundTripper, *dialDurationRecorder, func(), error) {
	recorder := &dialDurationRecorder{}
	parsed, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, nil, nil, err
	}

	switch cfg.Proto {
	case "h1", "http1", "http/1.1":
		transport := &http.Transport{
			DialContext:           tcpDialer(cfg, recorder),
			TLSClientConfig:       &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // local benchmark cert
			DisableKeepAlives:     true,
			ForceAttemptHTTP2:     false,
			TLSNextProto:          map[string]func(string, *tls.Conn) http.RoundTripper{},
			MaxIdleConnsPerHost:   1,
			ResponseHeaderTimeout: cfg.Timeout,
		}
		return transport, recorder, func() { transport.CloseIdleConnections() }, nil
	case "h2", "http2", "http/2":
		transport := &http.Transport{
			DialContext:           tcpDialer(cfg, recorder),
			TLSClientConfig:       &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // local benchmark cert
			DisableKeepAlives:     true,
			ForceAttemptHTTP2:     true,
			MaxIdleConnsPerHost:   1,
			ResponseHeaderTimeout: cfg.Timeout,
		}
		return transport, recorder, func() { transport.CloseIdleConnections() }, nil
	case "h3", "http3", "http/3":
		transport := &http3.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, //nolint:gosec // local benchmark cert
				ServerName:         parsed.Hostname(),
			},
			Dial: func(ctx context.Context, _ string, tlsCfg *tls.Config, quicCfg *quic.Config) (*quic.Conn, error) {
				recorder.Begin()
				addr := parsed.Host
				if cfg.DialAddr != "" {
					addr = cfg.DialAddr
				}
				addr = ensureUDPPort(addr, parsed)
				conn, err := quic.DialAddr(ctx, addr, tlsCfg, quicCfg)
				recorder.End()
				return conn, err
			},
		}
		return transport, recorder, func() { _ = transport.Close() }, nil
	default:
		return nil, nil, nil, fmt.Errorf("unsupported proto %q", cfg.Proto)
	}
}

func tcpDialer(cfg probeConfig, recorder *dialDurationRecorder) func(context.Context, string, string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: cfg.Timeout}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		target := address
		if cfg.DialAddr != "" {
			target = cfg.DialAddr
		}
		recorder.Begin()
		conn, err := dialer.DialContext(ctx, network, target)
		recorder.End()
		return conn, err
	}
}

func requestBody(n int) []byte {
	if n <= 0 {
		return nil
	}
	body := make([]byte, n)
	for i := range body {
		body[i] = randomPayload[i%len(randomPayload)]
	}
	return body
}

func readFirstByte(r io.Reader) (int64, error) {
	buf := make([]byte, 1)
	n, err := r.Read(buf)
	return int64(n), err
}

func readFirstSSEEvent(r io.Reader) (int64, error) {
	reader := bufio.NewReader(r)
	var total int64
	for {
		line, err := reader.ReadBytes('\n')
		total += int64(len(line))
		if len(bytes.TrimSpace(line)) == 0 && total > 0 {
			return total, err
		}
		if err != nil {
			return total, err
		}
	}
}

func printProbeSummary(samples []probeSample) int {
	okSamples := make([]probeSample, 0, len(samples))
	failed := 0
	for _, sample := range samples {
		if sample.ErrText != "" {
			failed++
			continue
		}
		okSamples = append(okSamples, sample)
	}
	fmt.Printf("Summary: total=%d ok=%d failed=%d\n", len(samples), len(okSamples), failed)
	if len(okSamples) == 0 {
		return failed
	}
	printDurationMetric("dial_ms", okSamples, func(s probeSample) time.Duration { return s.Dial })
	printDurationMetric("ttfb_ms", okSamples, func(s probeSample) time.Duration { return s.TTFB })
	printDurationMetric("total_ms", okSamples, func(s probeSample) time.Duration { return s.Total })
	printFloatMetric("bytes", okSamples, func(s probeSample) float64 { return float64(s.Bytes) })
	return failed
}

func printDurationMetric(label string, samples []probeSample, pick func(probeSample) time.Duration) {
	printFloatMetric(label, samples, func(s probeSample) float64 { return durationMillis(pick(s)) })
}

func printFloatMetric(label string, samples []probeSample, pick func(probeSample) float64) {
	values := make([]float64, 0, len(samples))
	for _, sample := range samples {
		values = append(values, pick(sample))
	}
	sort.Float64s(values)
	fmt.Printf("  %s avg=%.3f p50=%.3f p95=%.3f min=%.3f max=%.3f\n",
		label,
		average(values),
		percentile(values, 50),
		percentile(values, 95),
		values[0],
		values[len(values)-1],
	)
}

func average(values []float64) float64 {
	var sum float64
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 1 {
		return values[0]
	}
	idx := (p / 100) * float64(len(values)-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))
	if lower == upper {
		return values[lower]
	}
	weight := idx - float64(lower)
	return values[lower] + (values[upper]-values[lower])*weight
}

func durationMillis(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

func ensureUDPPort(addr string, parsed *url.URL) string {
	if _, _, err := net.SplitHostPort(addr); err == nil {
		return addr
	}

	host := addr
	port := parsed.Port()
	if port == "" {
		port = "443"
	}
	return net.JoinHostPort(host, port)
}
