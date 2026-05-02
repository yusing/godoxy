package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

var (
	printables    = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	randomPayload = make([]byte, 4096)
	jsonPayload   = mustJSON(map[string]any{
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
	wsUpgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
)

func init() {
	for i := range randomPayload {
		randomPayload[i] = printables[rand.IntN(len(printables))]
	}
}

func newBenchHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", rootHandler)
	mux.HandleFunc("/healthz", healthzHandler)
	mux.HandleFunc("/json", jsonHandler)
	mux.HandleFunc("/upload", uploadHandler)
	mux.HandleFunc("/stream", streamHandler)
	mux.HandleFunc("/sse", sseHandler)
	mux.HandleFunc("/ws", websocketHandler)
	return mux
}

func rootHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(randomPayload)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(randomPayload)
}

func healthzHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ok\n")
}

func jsonHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(jsonPayload)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(jsonPayload)
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		w.Header().Set("Allow", "POST, PUT")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	const maxUploadBytes = 16 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	defer r.Body.Close()

	received, err := io.Copy(io.Discard, r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("read body: %v", err), http.StatusBadRequest)
		return
	}

	resp := mustJSON(map[string]any{
		"ok":           true,
		"method":       r.Method,
		"received":     received,
		"content_type": r.Header.Get("Content-Type"),
	})
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(resp)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(resp)
}

func streamHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		unsupportedResponseWriter(w, "streaming")
		return
	}

	chunks := clampInt(queryInt(r, "chunks", 8), 1, 128)
	chunkBytes := clampInt(queryInt(r, "chunk_bytes", 4096), 1, len(randomPayload))
	interval := time.Duration(clampInt(queryInt(r, "interval_ms", 15), 0, 60_000)) * time.Millisecond

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	for range chunks {
		if _, err := w.Write(randomPayload[:chunkBytes]); err != nil {
			return
		}
		flusher.Flush()
		if interval > 0 {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(interval):
			}
		}
	}
}

func sseHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		unsupportedResponseWriter(w, "SSE")
		return
	}

	interval := time.Duration(clampInt(queryInt(r, "interval_ms", 150), 0, 60_000)) * time.Millisecond
	count := clampInt(queryInt(r, "count", 3), 1, 32)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	for i := range count {
		if _, err := fmt.Fprintf(w, "event: tick\ndata: {\"sequence\":%d,\"ts\":%q}\n\n", i, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return
		}
		flusher.Flush()
		if i == count-1 || interval <= 0 {
			continue
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(interval):
		}
	}
}

func websocketHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	welcome := fmt.Sprintf("{\"event\":\"welcome\",\"ts\":%q}", time.Now().UTC().Format(time.RFC3339Nano))
	if err := conn.WriteMessage(websocket.TextMessage, []byte(welcome)); err != nil {
		return
	}

	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if err := conn.WriteMessage(messageType, payload); err != nil {
			return
		}
	}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func queryInt(r *http.Request, key string, fallback int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

