package component

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type (
	// WriteFS is a file system that may be read from and written to
	WriteFS interface {
		OpenFile(name string, flag int, perm fs.FileMode) (io.WriteCloser, error)
	}

	// OSWriteFS implements WriteFS with the os file system
	OSWriteFS struct {
		Base string
	}
)

func NewOSWriteFS(base string) *OSWriteFS {
	return &OSWriteFS{
		Base: base,
	}
}

func (o *OSWriteFS) OpenFile(name string, flag int, perm fs.FileMode) (io.WriteCloser, error) {
	if !fs.ValidPath(name) {
		return nil, fs.ErrInvalid
	}
	path := filepath.Join(o.Base, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("Failed to mkdir: %w", err)
	}
	f, err := os.OpenFile(path, flag, perm)
	if err != nil {
		return nil, fmt.Errorf("Invalid file: %w", err)
	}
	return f, nil
}
