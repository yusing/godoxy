package accesslog

import (
	"bytes"
	"testing"
	"unsafe"
)

func TestBackScannerRetainsFinalLineUntilRelease(t *testing.T) {
	const (
		poolSize = 2 * 1024
		drain    = 512
	)

	borrowed := make([][]byte, drain)
	for i := range borrowed {
		borrowed[i] = sizedPool.GetSized(poolSize)
	}
	defer func() {
		for _, buf := range borrowed {
			sizedPool.Put(buf)
		}
	}()

	input := []byte("final line")
	scanner := newBackScanner(bytes.NewReader(input), int64(len(input)), make([]byte, poolSize/2))
	if !scanner.Scan() {
		t.Fatalf("Scan() = false, error = %v", scanner.Err())
	}
	line := scanner.Bytes()
	linePtr := unsafe.SliceData(line)

	checked := make([][]byte, drain)
	defer func() {
		for _, buf := range checked {
			if buf != nil {
				sizedPool.Put(buf)
			}
		}
	}()
	for i := range checked {
		checked[i] = sizedPool.GetSized(poolSize)
		if unsafe.SliceData(checked[i]) == linePtr {
			t.Fatal("final line returned to pool before Release")
		}
	}

	scanner.Release()
	if scanner.Bytes() != nil {
		t.Fatal("Bytes() remains accessible after Release")
	}
}
