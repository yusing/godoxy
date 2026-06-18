//go:build !aix && !android && !darwin && !dragonfly && !freebsd && !hurd && !illumos && !ios && !linux && !netbsd && !openbsd && !solaris

package accesslog

import "os"

type accessLogMetadata struct {
	mode os.FileMode
}

func captureAccessLogMetadata(_ *os.File, oldInfo os.FileInfo) (accessLogMetadata, error) {
	return accessLogMetadata{mode: oldInfo.Mode().Perm()}, nil
}

func (metadata accessLogMetadata) apply(path string) error {
	return os.Chmod(path, metadata.mode)
}
