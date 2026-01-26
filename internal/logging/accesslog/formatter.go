package accesslog

import (
	"bytes"
	"iter"
	"net"
	"net/http"
	"strconv"

	"github.com/rs/zerolog"
	maxmind "github.com/yusing/godoxy/internal/maxmind/types"
	"github.com/yusing/goutils/mockable"
)

type (
	CommonFormatter struct {
		cfg *Fields
	}
	CombinedFormatter   struct{ CommonFormatter }
	JSONFormatter       struct{ cfg *Fields }
	ConsoleFormatter    struct{ cfg *Fields }
	ACLLogFormatter     struct{}
	ConsoleACLFormatter struct{}
)

const LogTimeFormat = "02/Jan/2006:15:04:05 -0700"

func scheme(req *http.Request) string {
	if req.TLS != nil {
		return "https"
	}
	return "http"
}

func appendRequestURI(line *bytes.Buffer, req *http.Request, query iter.Seq2[string, []string]) {
	uri := req.URL.EscapedPath()
	line.WriteString(uri)
	isFirst := true
	for k, v := range query {
		if isFirst {
			line.WriteByte('?')
			isFirst = false
		} else {
			line.WriteByte('&')
		}
		for i, val := range v {
			if i > 0 {
				line.WriteByte('&')
			}
			line.WriteString(k)
			line.WriteByte('=')
			line.WriteString(val)
		}
	}
}

func clientIP(req *http.Request) string {
	clientIP, _, err := net.SplitHostPort(req.RemoteAddr)
	if err == nil {
		return clientIP
	}
	return req.RemoteAddr
}

func (f CommonFormatter) AppendRequestLog(line *bytes.Buffer, req *http.Request, res *http.Response) {
	query := f.cfg.Query.IterQuery(req.URL.Query())

	line.WriteString(req.Host)
	line.WriteByte(' ')

	line.WriteString(clientIP(req))
	line.WriteString(" - - [")

	line.WriteString(mockable.TimeNow().Format(LogTimeFormat))
	line.WriteString("] \"")

	line.WriteString(req.Method)
	line.WriteByte(' ')
	appendRequestURI(line, req, query)
	line.WriteByte(' ')
	line.WriteString(req.Proto)
	line.WriteByte('"')
	line.WriteByte(' ')

	line.WriteString(strconv.FormatInt(int64(res.StatusCode), 10))
	line.WriteByte(' ')
	line.WriteString(strconv.FormatInt(res.ContentLength, 10))
}

func (f CombinedFormatter) AppendRequestLog(line *bytes.Buffer, req *http.Request, res *http.Response) {
	f.CommonFormatter.AppendRequestLog(line, req, res)
	line.WriteString(" \"")
	line.WriteString(req.Referer())
	line.WriteString("\" \"")
	line.WriteString(req.UserAgent())
	line.WriteByte('"')
}

func (f JSONFormatter) AppendRequestLog(line *bytes.Buffer, req *http.Request, res *http.Response) {
	logger := zerolog.New(line)
	f.LogRequestZeroLog(&logger, req, res)
}

func (f JSONFormatter) LogRequestZeroLog(logger *zerolog.Logger, req *http.Request, res *http.Response) {
	query := f.cfg.Query.ZerologQuery(req.URL.Query())
	headers := f.cfg.Headers.ZerologHeaders(req.Header)
	cookies := f.cfg.Cookies.ZerologCookies(req.Cookies())
	contentType := res.Header.Get("Content-Type")

	event := logger.Info().
		Str("time", mockable.TimeNow().Format(LogTimeFormat)).
		Str("ip", clientIP(req)).
		Str("method", req.Method).
		Str("scheme", scheme(req)).
		Str("host", req.Host).
		Str("path", req.URL.Path).
		Str("protocol", req.Proto).
		Int("status", res.StatusCode).
		Str("type", contentType).
		Int64("size", res.ContentLength).
		Str("referer", req.Referer()).
		Str("useragent", req.UserAgent()).
		Object("query", query).
		Object("headers", headers).
		Object("cookies", cookies)

	// NOTE: zerolog will append a newline to the buffer
	event.Send()
}

func (f ConsoleFormatter) LogRequestZeroLog(logger *zerolog.Logger, req *http.Request, res *http.Response) {
	contentType := res.Header.Get("Content-Type")

	var reqURI bytes.Buffer
	appendRequestURI(&reqURI, req, f.cfg.Query.IterQuery(req.URL.Query()))

	event := logger.Info().
		Bytes("uri", reqURI.Bytes()).
		Str("protocol", req.Proto).
		Str("type", contentType).
		Int64("size", res.ContentLength).
		Str("useragent", req.UserAgent())

	// NOTE: zerolog will append a newline to the buffer
	event.Msgf("[%d] %s %s://%s from %s", res.StatusCode, req.Method, scheme(req), req.Host, clientIP(req))
}

func (f ACLLogFormatter) AppendACLLog(line *bytes.Buffer, info *maxmind.IPInfo, blocked bool) {
	logger := zerolog.New(line)
	f.LogACLZeroLog(&logger, info, blocked)
}

func (f ACLLogFormatter) LogACLZeroLog(logger *zerolog.Logger, info *maxmind.IPInfo, blocked bool) {
	event := logger.Info().
		Str("time", mockable.TimeNow().Format(LogTimeFormat)).
		Str("ip", info.Str)
	if blocked {
		event.Str("action", "block")
	} else {
		event.Str("action", "allow")
	}
	if info.City != nil {
		if isoCode := info.City.Country.IsoCode; isoCode != "" {
			event.Str("iso_code", isoCode)
		}
		if timeZone := info.City.Location.TimeZone; timeZone != "" {
			event.Str("time_zone", timeZone)
		}
	}
	// NOTE: zerolog will append a newline to the buffer
	event.Send()
}

func (f ConsoleACLFormatter) LogACLZeroLog(logger *zerolog.Logger, info *maxmind.IPInfo, blocked bool) {
	event := logger.Info()
	if info.City != nil {
		if isoCode := info.City.Country.IsoCode; isoCode != "" {
			event.Str("iso_code", isoCode)
		}
		if timeZone := info.City.Location.TimeZone; timeZone != "" {
			event.Str("time_zone", timeZone)
		}
	}
	action := "accepted"
	if blocked {
		action = "denied"
	}

	// NOTE: zerolog will append a newline to the buffer
	event.Msgf("request %s from %s", action, info.Str)
}
