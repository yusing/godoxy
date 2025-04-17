package certs

import (
	"testing"

	expect "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestZipCert(t *testing.T) {
	ca, crt, key := []byte("test1"), []byte("test2"), []byte("test3")
	zipData, err := ZipCert(ca, crt, key)
	expect.NoError(t, err)

	ca2, crt2, key2, err := ExtractCert(zipData)
	expect.NoError(t, err)
	expect.Equal(t, ca, ca2)
	expect.Equal(t, crt, crt2)
	expect.Equal(t, key, key2)
}
