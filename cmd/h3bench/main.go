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
	"net"
	"net/http"
	"net/url"
	"os"
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

func main() {
	_ = os.Setenv("QUIC_GO_DISABLE_RECEIVE_BUFFER_WARNING", "true")
	log.SetOutput(io.Discard)

	var (
		duration   = flag.Duration("d", 10*time.Second, "benchmark duration")
		concurrent = flag.Int("c", 100, "concurrent workers")
		host       = flag.String("host", "", "HTTP Host / TLS SNI override")
		insecure   = flag.Bool("k", true, "skip TLS certificate verification")
		method     = flag.String("method", http.MethodGet, "request method")
		dialAddr   = flag.String("dial", "", "UDP address to dial while keeping URL host for TLS/SNI")
	)
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: h3bench [flags] https://host:port/path\n")
		flag.PrintDefaults()
		os.Exit(2)
	}

	target := flag.Arg(0)
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		fmt.Fprintf(os.Stderr, "invalid target %q: HTTP/3 target must be https URL\n", target)
		os.Exit(2)
	}

	tlsCfg := &tls.Config{InsecureSkipVerify: *insecure} //nolint:gosec // benchmark tool supports local self-signed endpoints.
	if *host != "" {
		tlsCfg.ServerName = *host
	}

	transport := &http3.Transport{TLSClientConfig: tlsCfg}
	if *dialAddr != "" {
		transport.Dial = func(ctx context.Context, _ string, tlsCfg *tls.Config, cfg *quic.Config) (*quic.Conn, error) {
			udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
			if err != nil {
				return nil, err
			}
			udpAddr, err := net.ResolveUDPAddr("udp", *dialAddr)
			if err != nil {
				_ = udpConn.Close()
				return nil, err
			}
			tr := &quic.Transport{Conn: udpConn}
			conn, err := tr.DialEarly(ctx, udpAddr, tlsCfg, cfg)
			if err != nil {
				_ = tr.Close()
				return nil, err
			}
			return conn, nil
		}
	}
	defer transport.Close()

	client := &http.Client{Transport: transport}

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	results := make(chan result, max(1, *concurrent)*2)
	var wg sync.WaitGroup
	var started atomic.Int64

	start := time.Now()
	for range max(1, *concurrent) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				started.Add(1)
				r := doRequest(ctx, client, *method, target, *host)
				if r.err != nil {
					time.Sleep(10 * time.Millisecond)
				}
				results <- r
			}
		}()
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
	if sum.count == 0 && started.Load() > 0 {
		elapsed = *duration
	}
	sum.print(target, *duration, elapsed, max(1, *concurrent))
	if sum.count == 0 {
		os.Exit(1)
	}
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
		if errors.Is(r.err, context.Canceled) || errors.Is(r.err, context.DeadlineExceeded) || strings.Contains(r.err.Error(), "H3_REQUEST_CANCELLED") {
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

func (s *summary) print(target string, requested, elapsed time.Duration, concurrent int) {
	sort.Slice(s.latencies, func(i, j int) bool { return s.latencies[i] < s.latencies[j] })

	fmt.Println("HTTP/3 benchmark")
	fmt.Printf("URL: %s\n", target)
	fmt.Printf("Duration: %s (elapsed %s)\n", requested, elapsed.Round(time.Millisecond))
	fmt.Printf("Concurrency: %d\n", concurrent)
	fmt.Printf("Requests: %d\n", s.count)
	fmt.Printf("2xx: %d\n", s.status2xx)
	fmt.Printf("Failed: %d\n", s.failed)
	fmt.Printf("Throughput: %.2f req/s\n", float64(s.count)/elapsed.Seconds())
	fmt.Printf("Transfer: %.2f MiB/s\n", float64(s.bytes)/(1024*1024)/elapsed.Seconds())
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
