//go:build !linux

package accesslog

import "os"

func readAccessLogACL(_ *os.File) ([]byte, error) {
	return nil, nil
}

func applyAccessLogACL(_ string, _ []byte) error {
	return nil
}
