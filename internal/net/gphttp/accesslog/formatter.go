package accesslog

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/yusing/go-proxy/internal/logging"
)

type (
	CommonFormatter struct {
		cfg        *Fields
		GetTimeNow func() time.Time // for testing purposes only
	}
	CombinedFormatter struct{ CommonFormatter }
	JSONFormatter     struct{ CommonFormatter }

	JSONLogEntry struct {
		Time        string              `json:"time"`
		IP          string              `json:"ip"`
		Method      string              `json:"method"`
		Scheme      string              `json:"scheme"`
		Host        string              `json:"host"`
		URI         string              `json:"uri"`
		Protocol    string              `json:"protocol"`
		Status      int                 `json:"status"`
		Error       string              `json:"error,omitempty"`
		ContentType string              `json:"type"`
		Size        int64               `json:"size"`
		Referer     string              `json:"referer"`
		UserAgent   string              `json:"useragent"`
		Query       map[string][]string `json:"query,omitempty"`
		Headers     map[string][]string `json:"headers,omitempty"`
		Cookies     map[string]string   `json:"cookies,omitempty"`
	}
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

	entry := JSONLogEntry{
		Time:        f.GetTimeNow().Format(LogTimeFormat),
		IP:          clientIP(req),
		Method:      req.Method,
		Scheme:      scheme(req),
		Host:        req.Host,
		URI:         requestURI(req.URL, query),
		Protocol:    req.Proto,
		Status:      res.StatusCode,
		ContentType: res.Header.Get("Content-Type"),
		Size:        res.ContentLength,
		Referer:     req.Referer(),
		UserAgent:   req.UserAgent(),
		Query:       query,
		Headers:     headers,
		Cookies:     cookies,
	}

	if res.StatusCode >= 400 {
		entry.Error = res.Status
	}

	if entry.ContentType == "" {
		// try to get content type from request
		entry.ContentType = req.Header.Get("Content-Type")
	}

	marshaller := json.NewEncoder(line)
	err := marshaller.Encode(entry)
	if err != nil {
		logging.Err(err).Msg("failed to marshal json log")
	}
}
