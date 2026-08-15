package socketproxy

import (
	"bytes"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestDockerSocketHandlerPreservesFixedLengthResponse(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "docker.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}

	backend := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "2")
		_, _ = io.WriteString(w, "OK")
	}))
	backend.Listener = listener
	backend.Start()
	t.Cleanup(backend.Close)

	var serverLog bytes.Buffer
	proxy := httptest.NewUnstartedServer(dockerSocketHandler(socketPath))
	proxy.Config.ErrorLog = log.New(&serverLog, "", 0)
	proxy.Start()
	t.Cleanup(proxy.Close)

	resp, err := proxy.Client().Get(proxy.URL + "/_ping")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "OK" {
		t.Fatalf("body = %q, want %q", body, "OK")
	}
	if resp.ContentLength != 2 {
		t.Fatalf("Content-Length = %d, want 2", resp.ContentLength)
	}
	if len(resp.TransferEncoding) != 0 {
		t.Fatalf("Transfer-Encoding = %v, want none", resp.TransferEncoding)
	}
	if serverLog.Len() != 0 {
		t.Fatalf("socket proxy server log: %s", serverLog.String())
	}
}
