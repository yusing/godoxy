package migrations

import (
	"os"
	"path/filepath"
)

func mv(old, new string) error {
	if _, err := os.Stat(old); os.IsNotExist(err) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(new), 0o755); err != nil {
		return err
	}
	return os.Rename(old, new)
}
