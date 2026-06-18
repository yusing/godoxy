//go:build aix || android || darwin || dragonfly || freebsd || hurd || illumos || ios || linux || netbsd || openbsd || solaris

package accesslog

import (
	"fmt"
	"os"
	"syscall"
)

type accessLogMetadata struct {
	mode     os.FileMode
	uid      int
	gid      int
	hasOwner bool
	acl      []byte
}

func captureAccessLogMetadata(file *os.File, oldInfo os.FileInfo) (accessLogMetadata, error) {
	metadata := accessLogMetadata{mode: oldInfo.Mode().Perm()}
	oldStat, ok := oldInfo.Sys().(*syscall.Stat_t)
	if ok {
		metadata.uid = int(oldStat.Uid)
		metadata.gid = int(oldStat.Gid)
		metadata.hasOwner = true
	}

	acl, err := readAccessLogACL(file)
	if err != nil {
		return metadata, err
	}
	metadata.acl = acl
	return metadata, nil
}

func (metadata accessLogMetadata) apply(path string) error {
	newInfo, err := os.Stat(path)
	if err != nil {
		return err
	}
	newStat, ok := newInfo.Sys().(*syscall.Stat_t)
	if metadata.hasOwner && ok && (metadata.uid != int(newStat.Uid) || metadata.gid != int(newStat.Gid)) {
		if err := os.Chown(path, metadata.uid, metadata.gid); err != nil {
			return fmt.Errorf("access log metadata owner restore error: %w", err)
		}
	}

	if err := os.Chmod(path, metadata.mode); err != nil {
		return fmt.Errorf("access log metadata mode restore error: %w", err)
	}
	if err := applyAccessLogACL(path, metadata.acl); err != nil {
		return err
	}
	return nil
}
