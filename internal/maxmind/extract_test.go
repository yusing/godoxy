package maxmind

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestExtractSupportedDatabaseSizes(t *testing.T) {
	for _, tc := range []struct {
		name string
		size int64
	}{
		{"GeoLite2-Country.mmdb", 1 << 20},
		{"GeoLite2-City.mmdb", 64 << 20},
		{"GeoIP2-City.mmdb", 128 << 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var archive bytes.Buffer
			gz := gzip.NewWriter(&archive)
			tw := tar.NewWriter(gz)
			if err := tw.WriteHeader(&tar.Header{Name: "release/" + tc.name, Size: tc.size, Mode: 0600}); err != nil {
				t.Fatal(err)
			}
			if _, err := io.CopyN(tw, zeroReader{}, tc.size); err != nil {
				t.Fatal(err)
			}
			if err := tw.Close(); err != nil {
				t.Fatal(err)
			}
			if err := gz.Close(); err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(t.TempDir(), tc.name)
			if err := extractFileFromTarGz(archive.Bytes(), tc.name, dest); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(dest)
			if err != nil {
				t.Fatal(err)
			}
			if info.Size() != tc.size {
				t.Fatalf("size = %d, want %d", info.Size(), tc.size)
			}
		})
	}
}

func TestExtractRejectsOversizedArchiveBeforeWriting(t *testing.T) {
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "GeoIP2-City.mmdb", Size: maxArchiveSize + 1, Mode: 0600}); err != nil {
		t.Fatal(err)
	}
	// A header suffices: the size must be rejected before reading the body.
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "database.mmdb")
	err := extractFileFromTarGz(archive.Bytes(), "GeoIP2-City.mmdb", dest)
	if err == nil || !strings.Contains(err.Error(), "archive size exceeds") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("destination created: %v", err)
	}
}
