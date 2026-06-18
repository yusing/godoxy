//go:build linux

package accesslog

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

const accessLogACLXattr = "system.posix_acl_access"

func readAccessLogACL(file *os.File) ([]byte, error) {
	size, err := unix.Fgetxattr(int(file.Fd()), accessLogACLXattr, nil)
	if isMissingAccessLogACL(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("access log metadata acl read error: %w", err)
	}
	if size == 0 {
		return nil, nil
	}

	acl := make([]byte, size)
	if _, err := unix.Fgetxattr(int(file.Fd()), accessLogACLXattr, acl); err != nil {
		if isMissingAccessLogACL(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("access log metadata acl read error: %w", err)
	}
	return acl, nil
}

func applyAccessLogACL(path string, acl []byte) error {
	if len(acl) == 0 {
		return nil
	}
	if err := unix.Setxattr(path, accessLogACLXattr, acl, 0); err != nil {
		return fmt.Errorf("access log metadata acl restore error: %w", err)
	}
	return nil
}

func isMissingAccessLogACL(err error) bool {
	return errors.Is(err, unix.ENODATA) ||
		errors.Is(err, unix.ENOTSUP) ||
		errors.Is(err, unix.EOPNOTSUPP)
}
