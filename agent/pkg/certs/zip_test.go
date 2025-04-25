package certs_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/go-proxy/agent/pkg/certs"
)

func TestZipCert(t *testing.T) {
	ca, crt, key := []byte("test1"), []byte("test2"), []byte("test3")
	zipData, err := certs.ZipCert(ca, crt, key)
	require.NoError(t, err)

	ca2, crt2, key2, err := certs.ExtractCert(zipData)
	require.NoError(t, err)
	require.Equal(t, ca, ca2)
	require.Equal(t, crt, crt2)
	require.Equal(t, key, key2)
}
