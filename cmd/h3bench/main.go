package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

type result struct {
	dur   time.Duration
	bytes int64
	code  int
	err   error
}

type summary struct {
	count      int64
	failed     int64
	bytes      int64
	status2xx  int64
	latencies  []time.Duration
	statusCode map[int]int64
	errors     map[string]int64
}

type benchConfig struct {
	duration    time.Duration
	requests    int64
	connections int
	streams     int
	host        string
	insecure    bool
	method      string
	dialAddr    string
	target      string
}

func main() {
	_ = os.Setenv("QUIC_GO_DISABLE_RECEIVE_BUFFER_WARNING", "true")
	log.SetOutput(io.Discard)

	var cfg benchConfig
	flag.DurationVar(&cfg.duration, "d", 10*time.Second, "benchmark duration; ignored after -n requests complete")
	flag.Int64Var(&cfg.requests, "n", 0, "total requests across all connections; 0 means duration-based")
	flag.IntVar(&cfg.connections, "c", 100, "number of QUIC connections/clients, equivalent to h2load -c")
	flag.IntVar(&cfg.streams, "m", 1, "max concurrent request streams per connection, equivalent to h2load -m")
	flag.StringVar(&cfg.host, "host", "", "HTTP Host / TLS SNI override")
	flag.BoolVar(&cfg.insecure, "k", true, "skip TLS certificate verification")
	flag.StringVar(&cfg.method, "method", http.MethodGet, "request method")
	flag.StringVar(&cfg.dialAddr, "dial", "", "UDP address to dial while keeping URL host for TLS/SNI")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: h3bench [flags] https://host:port/path\n")
		flag.PrintDefaults()
		os.Exit(2)
	}

	cfg.target = flag.Arg(0)
	u, err := url.Parse(cfg.target)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		fmt.Fprintf(os.Stderr, "invalid target %q: HTTP/3 target must be https URL\n", cfg.target)
		os.Exit(2)
	}
	if cfg.connections <= 0 {
		fmt.Fprintln(os.Stderr, "-c must be greater than zero")
		os.Exit(2)
	}
	if cfg.streams <= 0 {
		fmt.Fprintln(os.Stderr, "-m must be greater than zero")
		os.Exit(2)
	}
	if cfg.requests < 0 {
		fmt.Fprintln(os.Stderr, "-n must be zero or greater")
		os.Exit(2)
	}
	if cfg.duration <= 0 && cfg.requests == 0 {
		fmt.Fprintln(os.Stderr, "-d must be greater than zero for duration-based benchmarks")
		os.Exit(2)
	}

	sum, elapsed := runBenchmark(cfg, u)
	sum.print(cfg, elapsed)
	if sum.count == 0 || sum.failed > 0 || sum.status2xx != sum.count {
		os.Exit(1)
	}
}

func runBenchmark(cfg benchConfig, u *url.URL) (summary, time.Duration) {
	ctx := context.Background()
	var cancel context.CancelFunc
	if cfg.requests == 0 {
		ctx, cancel = context.WithTimeout(ctx, cfg.duration)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	results := make(chan result, max(1, cfg.connections*cfg.streams)*2)
	var wg sync.WaitGroup
	var issued atomic.Int64
	var completed atomic.Int64
	var started atomic.Int64

	start := time.Now()
	for connID := range cfg.connections {
		client, closeTransport := newClient(cfg, u)
		wg.Go(func() {
			defer closeTransport()

			var streamWG sync.WaitGroup
			for streamID := 0; streamID < cfg.streams; streamID++ {
				streamWG.Go(func() {
					for {
						if ctx.Err() != nil {
							return
						}
						if cfg.requests > 0 && issued.Add(1) > cfg.requests {
							return
						}
						started.Add(1)
						r := doRequest(ctx, client, cfg.method, cfg.target, cfg.host)
						if cfg.requests > 0 && completed.Add(1) >= cfg.requests {
							cancel()
						}
						if r.err != nil {
							time.Sleep(10 * time.Millisecond)
						}
						if isBenignContextError(r.err) {
							return
						}
						results <- r
					}
				})
			}
			streamWG.Wait()
		})
		_ = connID
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	sum := summary{
		statusCode: make(map[int]int64),
		errors:     make(map[string]int64),
	}
	for r := range results {
		sum.add(r)
	}

	elapsed := time.Since(start)
	if sum.count == 0 && started.Load() > 0 && cfg.requests == 0 {
		elapsed = cfg.duration
	}
	return sum, elapsed
}

func newClient(cfg benchConfig, u *url.URL) (*http.Client, func()) {
	tlsCfg := &tls.Config{InsecureSkipVerify: cfg.insecure} //nolint:gosec // benchmark tool supports local self-signed endpoints.
	if cfg.host != "" {
		tlsCfg.ServerName = cfg.host
	}

	transport := &http3.Transport{TLSClientConfig: tlsCfg}
	transport.Dial = func(ctx context.Context, addr string, tlsCfg *tls.Config, qcfg *quic.Config) (*quic.Conn, error) {
		dialAddr := cfg.dialAddr
		if dialAddr == "" {
			dialAddr = addr
		}
		return quic.DialAddr(ctx, dialAddr, tlsCfg, qcfg)
	}
	_ = u
	return &http.Client{Transport: transport}, func() { _ = transport.Close() }
}

func doRequest(ctx context.Context, client *http.Client, method, target, host string) result {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return result{dur: time.Since(start), err: err}
	}
	if host != "" {
		req.Host = host
	}

	resp, err := client.Do(req)
	if err != nil {
		return result{dur: time.Since(start), err: err}
	}
	defer resp.Body.Close()

	n, err := io.Copy(io.Discard, resp.Body)
	return result{dur: time.Since(start), bytes: n, code: resp.StatusCode, err: err}
}

func (s *summary) add(r result) {
	if r.err != nil {
		if isBenignContextError(r.err) {
			return
		}
		s.failed++
		s.errors[r.err.Error()]++
		return
	}
	s.count++
	s.bytes += r.bytes
	s.statusCode[r.code]++
	if r.code >= 200 && r.code < 300 {
		s.status2xx++
	}
	s.latencies = append(s.latencies, r.dur)
}

func isBenignContextError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "H3_REQUEST_CANCELLED")
}

func (s *summary) print(cfg benchConfig, elapsed time.Duration) {
	slices.Sort(s.latencies)

	fmt.Println("HTTP/3 benchmark")
	fmt.Printf("URL: %s\n", cfg.target)
	fmt.Printf("Duration: %s (elapsed %s)\n", cfg.duration, elapsed.Round(time.Millisecond))
	if cfg.requests > 0 {
		fmt.Printf("Requested: %d\n", cfg.requests)
	}
	fmt.Printf("Connections: %d\n", cfg.connections)
	fmt.Printf("Streams per connection: %d\n", cfg.streams)
	fmt.Printf("Max in-flight streams: %d\n", cfg.connections*cfg.streams)
	fmt.Printf("Requests: %d\n", s.count)
	fmt.Printf("2xx: %d\n", s.status2xx)
	fmt.Printf("Failed: %d\n", s.failed)
	elapsedSeconds := elapsed.Seconds()
	if elapsedSeconds <= 0 {
		elapsedSeconds = 1
	}
	fmt.Printf("Throughput: %.2f req/s\n", float64(s.count)/elapsedSeconds)
	fmt.Printf("Transfer: %.2f MiB/s\n", float64(s.bytes)/(1024*1024)/elapsedSeconds)
	fmt.Printf("Latency avg: %s\n", avgDuration(s.latencies).Round(time.Microsecond))
	fmt.Printf("Latency p50: %s\n", percentile(s.latencies, 50).Round(time.Microsecond))
	fmt.Printf("Latency p90: %s\n", percentile(s.latencies, 90).Round(time.Microsecond))
	fmt.Printf("Latency p99: %s\n", percentile(s.latencies, 99).Round(time.Microsecond))

	if len(s.statusCode) > 0 {
		codes := make([]int, 0, len(s.statusCode))
		for code := range s.statusCode {
			codes = append(codes, code)
		}
		sort.Ints(codes)
		fmt.Print("Status codes:")
		for _, code := range codes {
			fmt.Printf(" %d=%d", code, s.statusCode[code])
		}
		fmt.Println()
	}

	if len(s.errors) > 0 {
		type errCount struct {
			err   string
			count int64
		}
		errs := make([]errCount, 0, len(s.errors))
		for err, count := range s.errors {
			errs = append(errs, errCount{err: err, count: count})
		}
		sort.Slice(errs, func(i, j int) bool { return errs[i].count > errs[j].count })
		fmt.Println("Errors:")
		for i, e := range errs {
			if i == 5 {
				break
			}
			fmt.Printf("  %d x %s\n", e.count, e.err)
		}
	}
}

func avgDuration(v []time.Duration) time.Duration {
	if len(v) == 0 {
		return 0
	}
	var total time.Duration
	for _, d := range v {
		total += d
	}
	return total / time.Duration(len(v))
}

func percentile(v []time.Duration, p float64) time.Duration {
	if len(v) == 0 {
		return 0
	}
	idx := int(math.Ceil((p/100)*float64(len(v)))) - 1
	return v[min(max(idx, 0), len(v)-1)]
}
