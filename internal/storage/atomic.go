// Package storage writes private state without leaving truncated files after a crash.
package storage

import (
	"os"
	"path/filepath"
)

func WriteFile(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".fly-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp, path)
}
