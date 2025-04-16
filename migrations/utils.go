package migrations

import (
	"os"
	"path/filepath"
)

func mv(old, new string) error {
	_, err := os.Stat(old)
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(new), 0o755); err != nil {
		return err
	}
	return os.Rename(old, new)
}
