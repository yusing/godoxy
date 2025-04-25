package accesslog

import (
	"bytes"
	"iter"
	"net"
	"net/http"
	"strconv"

	"github.com/rs/zerolog"
	acl "github.com/yusing/go-proxy/internal/acl/types"
	"github.com/yusing/go-proxy/internal/utils"
)

type (
	CommonFormatter struct {
		cfg *Fields
	}
	CombinedFormatter struct{ CommonFormatter }
	JSONFormatter     struct{ CommonFormatter }
	ACLLogFormatter   struct{}
)

const LogTimeFormat = "02/Jan/2006:15:04:05 -0700"

func scheme(req *http.Request) string {
	if req.TLS != nil {
		return "https"
	}
	return "http"
}

func appendRequestURI(line []byte, req *http.Request, query iter.Seq2[string, []string]) []byte {
	uri := req.URL.EscapedPath()
	line = append(line, uri...)
	isFirst := true
	for k, v := range query {
		if isFirst {
			line = append(line, '?')
			isFirst = false
		} else {
			line = append(line, '&')
		}
		line = append(line, k...)
		line = append(line, '=')
		for _, v := range v {
			line = append(line, v...)
		}
	}
	return line
}

func clientIP(req *http.Request) string {
	clientIP, _, err := net.SplitHostPort(req.RemoteAddr)
	if err == nil {
		return clientIP
	}
	return req.RemoteAddr
}

func (f *CommonFormatter) AppendRequestLog(line []byte, req *http.Request, res *http.Response) []byte {
	query := f.cfg.Query.IterQuery(req.URL.Query())

	line = append(line, req.Host...)
	line = append(line, ' ')

	line = append(line, clientIP(req)...)
	line = append(line, " - - ["...)

	line = utils.TimeNow().AppendFormat(line, LogTimeFormat)
	line = append(line, `] "`...)

	line = append(line, req.Method...)
	line = append(line, ' ')
	line = appendRequestURI(line, req, query)
	line = append(line, ' ')
	line = append(line, req.Proto...)
	line = append(line, '"')
	line = append(line, ' ')

	line = strconv.AppendInt(line, int64(res.StatusCode), 10)
	line = append(line, ' ')
	line = strconv.AppendInt(line, res.ContentLength, 10)
	return line
}

func (f *CombinedFormatter) AppendRequestLog(line []byte, req *http.Request, res *http.Response) []byte {
	line = f.CommonFormatter.AppendRequestLog(line, req, res)
	line = append(line, " \""...)
	line = append(line, req.Referer()...)
	line = append(line, "\" \""...)
	line = append(line, req.UserAgent()...)
	line = append(line, '"')
	return line
}

type zeroLogStringStringMapMarshaler struct {
	values map[string]string
}

func (z *zeroLogStringStringMapMarshaler) MarshalZerologObject(e *zerolog.Event) {
	if len(z.values) == 0 {
		return
	}
	for k, v := range z.values {
		e.Str(k, v)
	}
}

type zeroLogStringStringSliceMapMarshaler struct {
	values map[string][]string
}

func (z *zeroLogStringStringSliceMapMarshaler) MarshalZerologObject(e *zerolog.Event) {
	if len(z.values) == 0 {
		return
	}
	for k, v := range z.values {
		e.Strs(k, v)
	}
}

func (f *JSONFormatter) AppendRequestLog(line []byte, req *http.Request, res *http.Response) []byte {
	query := f.cfg.Query.ZerologQuery(req.URL.Query())
	headers := f.cfg.Headers.ZerologHeaders(req.Header)
	cookies := f.cfg.Cookies.ZerologCookies(req.Cookies())
	contentType := res.Header.Get("Content-Type")

	writer := bytes.NewBuffer(line)
	logger := zerolog.New(writer)
	event := logger.Info().
		Str("time", utils.TimeNow().Format(LogTimeFormat)).
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

	if res.StatusCode >= 400 {
		if res.Status != "" {
			event.Str("error", res.Status)
		} else {
			event.Str("error", http.StatusText(res.StatusCode))
		}
	}

	// NOTE: zerolog will append a newline to the buffer
	event.Send()
	return writer.Bytes()
}

func (f ACLLogFormatter) AppendACLLog(line []byte, info *acl.IPInfo, blocked bool) []byte {
	writer := bytes.NewBuffer(line)
	logger := zerolog.New(writer)
	event := logger.Info().
		Str("time", utils.TimeNow().Format(LogTimeFormat)).
		Str("ip", info.Str)
	if blocked {
		event.Str("action", "block")
	} else {
		event.Str("action", "allow")
	}
	if info.City != nil {
		event.Str("iso_code", info.City.Country.IsoCode)
		event.Str("time_zone", info.City.Location.TimeZone)
	}
	// NOTE: zerolog will append a newline to the buffer
	event.Send()
	return writer.Bytes()
}
