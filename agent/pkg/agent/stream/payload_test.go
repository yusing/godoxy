package stream

import (
	"bytes"
	"testing"
)

func TestStreamRequestHeader_RoundTripAndChecksum(t *testing.T) {
	h, err := NewStreamRequestHeader("example.com", "443")
	if err != nil {
		t.Fatalf("NewStreamRequestHeader: %v", err)
	}
	if !h.Validate() {
		t.Fatalf("expected header to validate")
	}

	var buf [headerSize]byte
	copy(buf[:], h.Bytes())
	h2 := ToHeader(buf)
	if !h2.Validate() {
		t.Fatalf("expected round-tripped header to validate")
	}
	host, port := h2.GetHostPort()
	if host != "example.com" || port != "443" {
		t.Fatalf("unexpected host/port: %q:%q", host, port)
	}
}

func TestStreamRequestPayload_WriteTo_WritesFullHeader(t *testing.T) {
	h, err := NewStreamRequestHeader("127.0.0.1", "53")
	if err != nil {
		t.Fatalf("NewStreamRequestHeader: %v", err)
	}

	p := &StreamRequestPayload{StreamRequestHeader: *h, Data: []byte("hello")}

	var out bytes.Buffer
	n, err := p.WriteTo(&out)
	if err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	if int(n) != headerSize+len(p.Data) {
		t.Fatalf("unexpected bytes written: got %d want %d", n, headerSize+len(p.Data))
	}

	written := out.Bytes()
	if len(written) != headerSize+len(p.Data) {
		t.Fatalf("unexpected output size: got %d", len(written))
	}
	if !bytes.Equal(written[:headerSize], h.Bytes()) {
		t.Fatalf("expected full header (including checksum) to be written")
	}
}
