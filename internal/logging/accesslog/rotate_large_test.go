package accesslog

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/yusing/goutils/mockable"
	strutils "github.com/yusing/goutils/strings"
	expect "github.com/yusing/goutils/testing"
	"golang.org/x/sys/unix"
)

func TestRotateOneGiBRealisticFreshAccessLog(t *testing.T) {
	if os.Getenv("ACCESSLOG_1GB_TEST") != "1" {
		t.Skip("set ACCESSLOG_1GB_TEST=1 to generate and rotate a 1GiB access-log fixture")
	}

	const size = 1 << 30

	now := expect.Must(time.Parse(time.RFC3339, "2024-01-31T03:04:05Z"))
	path := filepath.Join(t.TempDir(), "access.log")
	written := writeRealisticFreshAccessLog(t, path, now, size)

	file, err := os.OpenFile(path, os.O_RDWR, 0)
	expect.NoError(t, err)
	defer file.Close()

	mockable.MockTimeNow(now)
	var result RotateResult
	start := time.Now()
	rotated, err := rotateLogFile(file, &Retention{Days: 30}, &result)
	elapsed := time.Since(start)

	expect.NoError(t, err)
	expect.False(t, rotated)
	expect.Equal(t, result.OriginalSize, written)
	expect.Equal(t, result.NumBytesKeep, written)
	expect.Equal(t, result.NumLinesRead, 0)
	t.Logf("rotated %s fresh access log in %s", strutils.FormatByteSize(written), elapsed)
}

func writeRealisticFreshAccessLog(t *testing.T, path string, start time.Time, size int64) int64 {
	t.Helper()

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o644)
	expect.NoError(t, err)
	defer file.Close()

	mapSize := size + megabyte
	expect.NoError(t, file.Truncate(mapSize))

	data, err := unix.Mmap(int(file.Fd()), 0, int(mapSize), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	expect.NoError(t, err)

	buf := data[:0]
	var lineNo int64

	for len(buf) < int(size) {
		buf = appendRealisticCombinedLogLine(buf, start, lineNo)
		lineNo++
	}

	written := int64(len(buf))
	expect.NoError(t, unix.Msync(buf, unix.MS_SYNC))
	expect.NoError(t, unix.Munmap(data))
	expect.NoError(t, file.Truncate(written))

	t.Logf("wrote %s realistic access-log fixture (%d lines)", strutils.FormatByteSize(written), lineNo)
	return written
}

func appendRealisticCombinedLogLine(dst []byte, start time.Time, n int64) []byte {
	hosts := [...]string{
		"app.example.com",
		"api.example.com",
		"cdn.example.com",
		"admin.example.com",
		"assets.example.com",
	}
	methods := [...]string{"GET", "GET", "GET", "POST", "PUT", "DELETE", "HEAD"}
	paths := [...]string{
		"/",
		"/api/v1/users",
		"/api/v1/sessions",
		"/api/v1/projects",
		"/api/v1/events",
		"/dashboard",
		"/settings/profile",
		"/static/app.js",
		"/static/app.css",
		"/favicon.ico",
		"/healthz",
		"/metrics",
	}
	statuses := [...]int{200, 200, 200, 200, 201, 204, 301, 304, 400, 401, 403, 404, 429, 500, 502}
	referers := [...]string{
		"-",
		"https://www.google.com/",
		"https://github.com/",
		"https://app.example.com/dashboard",
		"https://app.example.com/settings/profile",
	}
	userAgents := [...]string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/125.0 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/605.1.15 Version/17.5 Safari/605.1.15",
		"Mozilla/5.0 (X11; Linux x86_64) Firefox/126.0",
		"curl/8.8.0",
		"Prometheus/2.52.0",
		"godoxy-healthcheck/1.0",
	}

	ts := start.Add(time.Duration(n%3600) * time.Second)
	host := hosts[n%int64(len(hosts))]
	method := methods[(n/3)%int64(len(methods))]
	path := paths[(n/7)%int64(len(paths))]
	status := statuses[(n/11)%int64(len(statuses))]
	referer := referers[(n/13)%int64(len(referers))]
	userAgent := userAgents[(n/17)%int64(len(userAgents))]
	ip := fmt.Sprintf("10.%d.%d.%d", (n/65536)%256, (n/256)%256, n%256)
	size := 128 + (n*7919)%2_000_000

	dst = append(dst, host...)
	dst = append(dst, ' ')
	dst = append(dst, ip...)
	dst = append(dst, " - - ["...)
	dst = ts.AppendFormat(dst, LogTimeFormat)
	dst = append(dst, `] "`...)
	dst = append(dst, method...)
	dst = append(dst, ' ')
	dst = append(dst, path...)
	dst = append(dst, "?request_id="...)
	dst = strconv.AppendInt(dst, n, 16)
	dst = append(dst, "&page="...)
	dst = strconv.AppendInt(dst, n%200, 10)
	dst = append(dst, "&cache_bust="...)
	dst = strconv.AppendInt(dst, (n*1103515245+12345)&0xffff, 10)
	dst = append(dst, ` HTTP/1.1" `...)
	dst = strconv.AppendInt(dst, int64(status), 10)
	dst = append(dst, ' ')
	dst = strconv.AppendInt(dst, size, 10)
	dst = append(dst, ` "`...)
	dst = append(dst, referer...)
	dst = append(dst, `" "`...)
	dst = append(dst, userAgent...)
	dst = append(dst, '"', '\n')
	return dst
}
