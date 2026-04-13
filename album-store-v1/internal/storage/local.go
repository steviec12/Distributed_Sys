package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LocalTempStorage manages transient upload files on the shared app/worker host.
type LocalTempStorage struct {
	dir string
}

// NewLocalTempStorage creates a temp-file helper rooted at dir or the OS temp dir.
func NewLocalTempStorage(dir string) *LocalTempStorage {
	if dir == "" {
		dir = os.TempDir()
	}

	return &LocalTempStorage{dir: dir}
}

// Save writes one upload body to a unique temp path and returns that path.
func (s *LocalTempStorage) Save(body io.Reader, filename string) (string, error) {
	file, err := os.CreateTemp(s.dir, "album-photo-*"+filepath.Ext(filename))
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}

	path := file.Name()
	if _, err := io.Copy(file, body); err != nil {
		file.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("write temp file: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close temp file: %w", err)
	}

	return path, nil
}

// Open returns a readable handle for one temp upload file.
func (s *LocalTempStorage) Open(path string) (io.ReadCloser, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open temp file: %w", err)
	}

	return file, nil
}

// Delete removes a temp upload file. Missing files are treated as already cleaned up.
func (s *LocalTempStorage) Delete(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete temp file: %w", err)
	}

	return nil
}
