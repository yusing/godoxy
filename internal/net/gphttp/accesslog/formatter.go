package accesslog

import (
	"bytes"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"github.com/yusing/go-proxy/pkg/json"
)

type (
	CommonFormatter struct {
		cfg        *Fields
		GetTimeNow func() time.Time // for testing purposes only
	}
	CombinedFormatter struct{ CommonFormatter }
	JSONFormatter     struct{ CommonFormatter }
)

const LogTimeFormat = "02/Jan/2006:15:04:05 -0700"

func scheme(req *http.Request) string {
	if req.TLS != nil {
		return "https"
	}
	return "http"
}

func requestURI(u *url.URL, query url.Values) string {
	uri := u.EscapedPath()
	if len(query) > 0 {
		uri += "?" + query.Encode()
	}
	return uri
}

func clientIP(req *http.Request) string {
	clientIP, _, err := net.SplitHostPort(req.RemoteAddr)
	if err == nil {
		return clientIP
	}
	return req.RemoteAddr
}

// debug only.
func (f *CommonFormatter) SetGetTimeNow(getTimeNow func() time.Time) {
	f.GetTimeNow = getTimeNow
}

func (f *CommonFormatter) Format(line *bytes.Buffer, req *http.Request, res *http.Response) {
	query := f.cfg.Query.ProcessQuery(req.URL.Query())

	line.WriteString(req.Host)
	line.WriteRune(' ')

	line.WriteString(clientIP(req))
	line.WriteString(" - - [")

	line.WriteString(f.GetTimeNow().Format(LogTimeFormat))
	line.WriteString("] \"")

	line.WriteString(req.Method)
	line.WriteRune(' ')
	line.WriteString(requestURI(req.URL, query))
	line.WriteRune(' ')
	line.WriteString(req.Proto)
	line.WriteString("\" ")

	line.WriteString(strconv.Itoa(res.StatusCode))
	line.WriteRune(' ')
	line.WriteString(strconv.FormatInt(res.ContentLength, 10))
}

func (f *CombinedFormatter) Format(line *bytes.Buffer, req *http.Request, res *http.Response) {
	f.CommonFormatter.Format(line, req, res)
	line.WriteString(" \"")
	line.WriteString(req.Referer())
	line.WriteString("\" \"")
	line.WriteString(req.UserAgent())
	line.WriteRune('"')
}

func (f *JSONFormatter) Format(line *bytes.Buffer, req *http.Request, res *http.Response) {
	query := f.cfg.Query.ProcessQuery(req.URL.Query())
	headers := f.cfg.Headers.ProcessHeaders(req.Header)
	headers.Del("Cookie")
	cookies := f.cfg.Cookies.ProcessCookies(req.Cookies())
	contentType := res.Header.Get("Content-Type")

	queryBytes, _ := json.Marshal(query)
	headersBytes, _ := json.Marshal(headers)
	cookiesBytes, _ := json.Marshal(cookies)

	logger := zerolog.New(line).With().Logger()
	event := logger.Info().
		Str("time", f.GetTimeNow().Format(LogTimeFormat)).
		Str("ip", clientIP(req)).
		Str("method", req.Method).
		Str("scheme", scheme(req)).
		Str("host", req.Host).
		Str("uri", requestURI(req.URL, query)).
		Str("protocol", req.Proto).
		Int("status", res.StatusCode).
		Str("type", contentType).
		Int64("size", res.ContentLength).
		Str("referer", req.Referer()).
		Str("useragent", req.UserAgent()).
		RawJSON("query", queryBytes).
		RawJSON("headers", headersBytes).
		RawJSON("cookies", cookiesBytes)

	if res.StatusCode >= 400 {
		if res.Status != "" {
			event.Str("error", res.Status)
		} else {
			event.Str("error", http.StatusText(res.StatusCode))
		}
	}
	event.Send()
}
