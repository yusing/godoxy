package main

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"time"
)

const (
	printables           = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	benchmarkPayloadSize = 4096
	readBufferBytes      = 32 * 1024
	maxHeaderLineBytes   = 16 << 10
	maxUploadBytes       = 16 << 20
	maxWSFrameBytes      = 1 << 20
	websocketGUID        = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
)

var (
	headerContentLength = []byte("content-length")
	headerContentType   = []byte("content-type")
	headerConnection    = []byte("connection")
	headerUpgrade       = []byte("upgrade")
	headerWSKey         = []byte("sec-websocket-key")
	headerClose         = []byte("close")
	headerWebSocket     = []byte("websocket")

	routePathRoot    = []byte("/")
	routePathHealthz = []byte("/healthz")
	routePathJSON    = []byte("/json")
	routePathUpload  = []byte("/upload")
	routePathStream  = []byte("/stream")
	routePathSSE     = []byte("/sse")
)

var (
	randomPayload = make([]byte, benchmarkPayloadSize)
	okBody        = []byte("ok\n")
	notFoundBody  = []byte("not found\n")
	badReqBody    = []byte("bad request\n")
	methodBody    = []byte("method not allowed\n")
	missingWSKey  = []byte("missing websocket key\n")

	jsonPayload = mustJSON(map[string]any{
		"service":   "bench",
		"version":   1,
		"ok":        true,
		"generated": "static-fixture",
		"items": []map[string]any{
			{"id": 1, "name": "alpha", "tags": []string{"proxy", "cache", "bench"}, "score": 0.98},
			{"id": 2, "name": "beta", "tags": []string{"stream", "sse", "json"}, "score": 0.92},
			{"id": 3, "name": "gamma", "tags": []string{"upload", "ws", "latency"}, "score": 0.89},
		},
		"meta": map[string]any{
			"region":     "lab",
			"cache":      false,
			"request_id": "bench-static",
			"payload":    "This endpoint mimics a small JSON API response with nested fields and repeated strings for proxy benchmarks.",
		},
	})

	rootResponseKeepAlive     []byte
	rootResponseClose         []byte
	healthzResponseKeepAlive  []byte
	healthzResponseClose      []byte
	jsonResponseKeepAlive     []byte
	jsonResponseClose         []byte
	notFoundResponseKeepAlive []byte
	notFoundResponseClose     []byte
	badReqResponseClose       []byte
	missingWSKeyResponseClose []byte
)

func init() {
	for i := range randomPayload {
		randomPayload[i] = printables[rand.IntN(len(printables))]
	}

	rootResponseKeepAlive = buildResponse(http.StatusOK, "application/octet-stream", randomPayload, true)
	rootResponseClose = buildResponse(http.StatusOK, "application/octet-stream", randomPayload, false)
	healthzResponseKeepAlive = buildResponse(http.StatusOK, "text/plain; charset=utf-8", okBody, true)
	healthzResponseClose = buildResponse(http.StatusOK, "text/plain; charset=utf-8", okBody, false)
	jsonResponseKeepAlive = buildResponse(http.StatusOK, "application/json", jsonPayload, true)
	jsonResponseClose = buildResponse(http.StatusOK, "application/json", jsonPayload, false)
	notFoundResponseKeepAlive = buildResponse(http.StatusNotFound, "text/plain; charset=utf-8", notFoundBody, true)
	notFoundResponseClose = buildResponse(http.StatusNotFound, "text/plain; charset=utf-8", notFoundBody, false)
	badReqResponseClose = buildResponse(http.StatusBadRequest, "text/plain; charset=utf-8", badReqBody, false)
	missingWSKeyResponseClose = buildResponse(http.StatusBadRequest, "text/plain; charset=utf-8", missingWSKey, false)
}

func serveTCP(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()

	log.Printf("bench tcp server listening on %s", addr)
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go handleConn(conn)
	}
}

type routeKind uint8

const (
	routeNotFound routeKind = iota
	routeRoot
	routeHealthz
	routeJSON
	routeUpload
	routeStream
	routeSSE
)

type rawRequest struct {
	Method        string
	Route         routeKind
	Query         []byte
	ContentLength int64
	ContentType   string
	WebSocketKey  string
	KeepAlive     bool
	WebSocket     bool
}

func handleConn(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReaderSize(conn, readBufferBytes)
	for {
		req, err := readRawRequest(reader)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				_, _ = conn.Write(badReqResponseClose)
			}
			return
		}

		if req.WebSocket {
			handleWebSocket(conn, reader, req)
			return
		}

		handleRawHTTP(conn, req)
		if !req.KeepAlive {
			return
		}
	}
}

func readRawRequest(reader *bufio.Reader) (rawRequest, error) {
	line, err := readHeaderLine(reader)
	if err != nil {
		return rawRequest{}, err
	}
	method, target, ok := parseRequestLine(line)
	if !ok {
		return rawRequest{}, errors.New("malformed request line")
	}

	req := rawRequest{
		KeepAlive: true,
	}
	path := target
	if pathOnly, query, ok := bytes.Cut(target, []byte{'?'}); ok {
		path = pathOnly
		req.Query = append(req.Query, query...)
	}
	req.Route = parseRoute(path)
	if req.Route == routeUpload {
		req.Method = string(method)
	}

	for {
		line, err = readHeaderLine(reader)
		if err != nil {
			return rawRequest{}, err
		}
		if len(line) == 0 {
			break
		}
		key, value, ok := bytes.Cut(line, []byte{':'})
		if !ok {
			continue
		}
		value = trimHeaderSpace(value)
		switch {
		case asciiEqualFold(key, headerContentLength):
			contentLength, err := parseContentLengthBytes(value)
			if err != nil {
				return rawRequest{}, err
			}
			req.ContentLength = contentLength
		case asciiEqualFold(key, headerContentType):
			req.ContentType = string(value)
		case asciiEqualFold(key, headerConnection):
			if containsToken(value, headerClose) {
				req.KeepAlive = false
			}
		case asciiEqualFold(key, headerUpgrade):
			req.WebSocket = asciiEqualFold(value, headerWebSocket)
		case asciiEqualFold(key, headerWSKey):
			req.WebSocketKey = string(value)
		}
	}

	if req.ContentLength > maxUploadBytes {
		return rawRequest{}, errors.New("request body too large")
	}
	if req.ContentLength > 0 {
		if _, err := io.CopyN(io.Discard, reader, req.ContentLength); err != nil {
			return rawRequest{}, err
		}
	}
	return req, nil
}

func readHeaderLine(reader *bufio.Reader) ([]byte, error) {
	line, err := reader.ReadSlice('\n')
	if errors.Is(err, bufio.ErrBufferFull) || len(line) > maxHeaderLineBytes {
		return nil, errors.New("header line too large")
	}
	if err != nil {
		return nil, err
	}
	return trimCRLF(line), nil
}

func parseRequestLine(line []byte) (method, target []byte, ok bool) {
	method, rest, ok := bytes.Cut(line, []byte{' '})
	if !ok || len(method) == 0 {
		return nil, nil, false
	}
	target, _, ok = bytes.Cut(rest, []byte{' '})
	if !ok || len(target) == 0 || target[0] != '/' {
		return nil, nil, false
	}
	return method, target, true
}

func parseContentLengthBytes(raw []byte) (int64, error) {
	if len(raw) == 0 {
		return 0, nil
	}
	var n int64
	for _, b := range raw {
		if b < '0' || b > '9' {
			return 0, errors.New("invalid content-length")
		}
		n = n*10 + int64(b-'0')
		if n > maxUploadBytes {
			return n, nil
		}
	}
	return n, nil
}

func trimCRLF(line []byte) []byte {
	line = bytes.TrimSuffix(line, []byte{'\n'})
	return bytes.TrimSuffix(line, []byte{'\r'})
}

func trimHeaderSpace(value []byte) []byte {
	for len(value) > 0 && (value[0] == ' ' || value[0] == '\t') {
		value = value[1:]
	}
	for len(value) > 0 && (value[len(value)-1] == ' ' || value[len(value)-1] == '\t') {
		value = value[:len(value)-1]
	}
	return value
}

func asciiEqualFold(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

func containsToken(value, token []byte) bool {
	for len(value) > 0 {
		value = bytes.TrimLeft(value, " \t,")
		part, rest, _ := bytes.Cut(value, []byte{','})
		if asciiEqualFold(trimHeaderSpace(part), token) {
			return true
		}
		if len(rest) == 0 {
			return false
		}
		value = rest
	}
	return false
}

func parseRoute(path []byte) routeKind {
	switch {
	case bytes.Equal(path, routePathRoot):
		return routeRoot
	case bytes.Equal(path, routePathHealthz):
		return routeHealthz
	case bytes.Equal(path, routePathJSON):
		return routeJSON
	case bytes.Equal(path, routePathUpload):
		return routeUpload
	case bytes.Equal(path, routePathStream):
		return routeStream
	case bytes.Equal(path, routePathSSE):
		return routeSSE
	default:
		return routeNotFound
	}
}

func handleRawHTTP(w io.Writer, req rawRequest) {
	switch req.Route {
	case routeRoot:
		writeCached(w, rootResponseKeepAlive, rootResponseClose, req.KeepAlive)
	case routeHealthz:
		writeCached(w, healthzResponseKeepAlive, healthzResponseClose, req.KeepAlive)
	case routeJSON:
		writeCached(w, jsonResponseKeepAlive, jsonResponseClose, req.KeepAlive)
	case routeUpload:
		writeUpload(w, req)
	case routeStream:
		writeStream(w, req)
	case routeSSE:
		writeSSE(w, req)
	default:
		writeCached(w, notFoundResponseKeepAlive, notFoundResponseClose, req.KeepAlive)
	}
}

func writeCached(w io.Writer, keepAliveResponse, closeResponse []byte, keepAlive bool) {
	if keepAlive {
		_, _ = w.Write(keepAliveResponse)
		return
	}
	_, _ = w.Write(closeResponse)
}

func buildResponse(status int, contentType string, body []byte, keepAlive bool) []byte {
	resp := appendResponseHeader(make([]byte, 0, 128+len(body)), status, contentType, len(body), keepAlive)
	return append(resp, body...)
}

func writeUpload(w io.Writer, req rawRequest) {
	if req.Method != http.MethodPost && req.Method != http.MethodPut {
		writeBytes(w, http.StatusMethodNotAllowed, "text/plain; charset=utf-8", methodBody, req.KeepAlive)
		return
	}
	resp := appendUploadResponse(make([]byte, 0, 96+len(req.Method)+len(req.ContentType)), req)
	writeBytes(w, http.StatusOK, "application/json", resp, req.KeepAlive)
}

func appendUploadResponse(dst []byte, req rawRequest) []byte {
	dst = append(dst, `{"ok":true,"method":`...)
	dst = strconv.AppendQuote(dst, req.Method)
	dst = append(dst, `,"received":`...)
	dst = strconv.AppendInt(dst, req.ContentLength, 10)
	if req.ContentType != "" {
		dst = append(dst, `,"content_type":`...)
		dst = strconv.AppendQuote(dst, req.ContentType)
	}
	dst = append(dst, '}')
	return dst
}

func writeBytes(w io.Writer, status int, contentType string, body []byte, keepAlive bool) {
	resp := buildResponse(status, contentType, body, keepAlive)
	_, _ = w.Write(resp)
}

func appendResponseHeader(dst []byte, status int, contentType string, contentLength int, keepAlive bool) []byte {
	dst = append(dst, "HTTP/1.1 "...)
	dst = strconv.AppendInt(dst, int64(status), 10)
	dst = append(dst, ' ')
	dst = append(dst, http.StatusText(status)...)
	dst = append(dst, "\r\nContent-Type: "...)
	dst = append(dst, contentType...)
	dst = append(dst, "\r\nContent-Length: "...)
	dst = strconv.AppendInt(dst, int64(contentLength), 10)
	dst = appendConnectionHeader(dst, keepAlive)
	return append(dst, "\r\n\r\n"...)
}

func writeStream(w io.Writer, req rawRequest) {
	chunks := clampInt(queryInt(req.Query, "chunks", 8), 1, 128)
	chunkBytes := clampInt(queryInt(req.Query, "chunk_bytes", benchmarkPayloadSize), 1, len(randomPayload))
	interval := time.Duration(clampInt(queryInt(req.Query, "interval_ms", 15), 0, 60_000)) * time.Millisecond
	writeChunkedHeader(w, "application/octet-stream", req.KeepAlive)
	for i := range chunks {
		writeChunk(w, randomPayload[:chunkBytes])
		if interval > 0 && i != chunks-1 {
			time.Sleep(interval)
		}
	}
	writeChunk(w, nil)
}

func writeSSE(w io.Writer, req rawRequest) {
	count := clampInt(queryInt(req.Query, "count", 3), 1, 32)
	interval := time.Duration(clampInt(queryInt(req.Query, "interval_ms", 150), 0, 60_000)) * time.Millisecond
	writeChunkedHeader(w, "text/event-stream", req.KeepAlive)
	for i := range count {
		writeChunk(w, appendSSEEvent(nil, i, time.Now().UTC()))
		if interval > 0 && i != count-1 {
			time.Sleep(interval)
		}
	}
	writeChunk(w, nil)
}

func appendSSEEvent(dst []byte, sequence int, ts time.Time) []byte {
	dst = append(dst, "event: tick\ndata: {\"sequence\":"...)
	dst = strconv.AppendInt(dst, int64(sequence), 10)
	dst = append(dst, ",\"ts\":\""...)
	dst = ts.AppendFormat(dst, time.RFC3339Nano)
	dst = append(dst, "\"}\n\n"...)
	return dst
}

func writeChunkedHeader(w io.Writer, contentType string, keepAlive bool) {
	header := appendChunkedHeader(nil, contentType, keepAlive)
	_, _ = w.Write(header)
}

func appendChunkedHeader(dst []byte, contentType string, keepAlive bool) []byte {
	dst = append(dst, "HTTP/1.1 200 OK\r\nContent-Type: "...)
	dst = append(dst, contentType...)
	dst = append(dst, "\r\nTransfer-Encoding: chunked"...)
	dst = appendConnectionHeader(dst, keepAlive)
	dst = append(dst, "\r\nCache-Control: no-cache\r\n\r\n"...)
	return dst
}

func appendConnectionHeader(dst []byte, keepAlive bool) []byte {
	if keepAlive {
		return append(dst, "\r\nConnection: keep-alive"...)
	}
	return append(dst, "\r\nConnection: close"...)
}

func writeChunk(w io.Writer, data []byte) {
	var header [18]byte
	chunkHeader := strconv.AppendInt(header[:0], int64(len(data)), 16)
	chunkHeader = append(chunkHeader, '\r', '\n')
	_, _ = w.Write(chunkHeader)
	if len(data) > 0 {
		_, _ = w.Write(data)
	}
	_, _ = io.WriteString(w, "\r\n")
}

func handleWebSocket(conn net.Conn, reader *bufio.Reader, req rawRequest) {
	if req.WebSocketKey == "" {
		_, _ = conn.Write(missingWSKeyResponseClose)
		return
	}
	writeWSHandshake(conn, req.WebSocketKey)
	writeWSFrame(conn, appendWSWelcome(nil, time.Now().UTC()))
	for {
		opcode, payload, err := readWSFrame(reader)
		if err != nil {
			return
		}
		switch opcode {
		case 0x8:
			return
		case 0x9:
			writeWSFrameWithOpcode(conn, 0xA, payload)
		case 0x1, 0x2:
			writeWSFrameWithOpcode(conn, opcode, payload)
		}
	}
}

func writeWSHandshake(w io.Writer, key string) {
	header := appendWSHandshake(nil, key)
	_, _ = w.Write(header)
}

func appendWSHandshake(dst []byte, key string) []byte {
	dst = append(dst, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: "...)
	dst = append(dst, websocketAccept(key)...)
	dst = append(dst, "\r\n\r\n"...)
	return dst
}

func appendWSWelcome(dst []byte, ts time.Time) []byte {
	dst = append(dst, "{\"event\":\"welcome\",\"ts\":\""...)
	dst = ts.AppendFormat(dst, time.RFC3339Nano)
	dst = append(dst, "\"}"...)
	return dst
}

func websocketAccept(key string) string {
	h := sha1.Sum([]byte(key + websocketGUID))
	return base64.StdEncoding.EncodeToString(h[:])
}

func readWSFrame(r *bufio.Reader) (byte, []byte, error) {
	b0, err := r.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	b1, err := r.ReadByte()
	if err != nil {
		return 0, nil, err
	}

	opcode := b0 & 0x0f
	masked := b1&0x80 != 0
	length := uint64(b1 & 0x7f)
	switch length {
	case 126:
		var buf [2]byte
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(buf[:]))
	case 127:
		var buf [8]byte
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return 0, nil, err
		}
		length = binary.BigEndian.Uint64(buf[:])
	}
	if length > maxWSFrameBytes {
		return 0, nil, errors.New("websocket frame too large")
	}

	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(r, mask[:]); err != nil {
			return 0, nil, err
		}
	}
	payload := make([]byte, int(length))
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%len(mask)]
		}
	}
	return opcode, payload, nil
}

func writeWSFrame(w io.Writer, payload []byte) {
	writeWSFrameWithOpcode(w, 0x1, payload)
}

func writeWSFrameWithOpcode(w io.Writer, opcode byte, payload []byte) {
	header := appendWSFrameHeader(nil, opcode, len(payload))
	_, _ = w.Write(header)
	_, _ = w.Write(payload)
}

func appendWSFrameHeader(dst []byte, opcode byte, length int) []byte {
	dst = append(dst, 0x80|opcode)
	switch {
	case length < 126:
		dst = append(dst, byte(length))
	case length <= 0xffff:
		dst = append(dst, 126, 0, 0)
		binary.BigEndian.PutUint16(dst[len(dst)-2:], uint16(length))
	default:
		dst = append(dst, 127, 0, 0, 0, 0, 0, 0, 0, 0)
		binary.BigEndian.PutUint64(dst[len(dst)-8:], uint64(length))
	}
	return dst
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func queryInt(q []byte, key string, fallback int) int {
	if len(q) == 0 {
		return fallback
	}
	keyBytes := []byte(key)
	for part := range bytes.SplitSeq(q, []byte{'&'}) {
		name, value, ok := bytes.Cut(part, []byte{'='})
		if !ok || !bytes.Equal(name, keyBytes) {
			continue
		}
		return atoiBytes(value, fallback)
	}
	return fallback
}

func atoiBytes(raw []byte, fallback int) int {
	if len(raw) == 0 {
		return fallback
	}
	var n int
	for _, b := range raw {
		if b < '0' || b > '9' {
			return fallback
		}
		n = n*10 + int(b-'0')
	}
	return n
}

func clampInt(v, lower, upper int) int {
	return min(max(v, lower), upper)
}
