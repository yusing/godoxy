package stream

import (
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
