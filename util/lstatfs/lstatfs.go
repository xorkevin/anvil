package lstatfs

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

var ErrNotImplemented = errors.New("not implemented")

type (
	LstatFS interface {
		fs.FS
		// Lstat returns the FileInfo of the named file without following symbolic
		// links
		Lstat(name string) (fs.FileInfo, error)
	}
)

// Lstat returns the FileInfo of the named file without following symbolic
// links
//
// If fsys does not implement LstatFS, then Lstat returns an error.
func Lstat(fsys fs.FS, name string) (fs.FileInfo, error) {
	rl, ok := fsys.(LstatFS)
	if !ok {
		return nil, &fs.PathError{Op: "lstat", Path: name, Err: ErrNotImplemented}
	}
	return rl.Lstat(name)
}

type (
	lstatFS struct {
		fsys fs.FS
		dir  string
	}
)

func (f *lstatFS) Open(name string) (fs.File, error) {
	return f.fsys.Open(name)
}

func (f *lstatFS) Stat(name string) (fs.FileInfo, error) {
	return fs.Stat(f.fsys, name)
}

func (f *lstatFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(f.fsys, name)
}

func (f *lstatFS) ReadFile(name string) ([]byte, error) {
	return fs.ReadFile(f.fsys, name)
}

func (f *lstatFS) Glob(pattern string) ([]string, error) {
	return fs.Glob(f.fsys, pattern)
}

func (f *lstatFS) Sub(dir string) (fs.FS, error) {
	fsys, err := fs.Sub(f.fsys, dir)
	if err != nil {
		return nil, err
	}
	return New(fsys, path.Join(f.dir, dir)), nil
}

func (f *lstatFS) Lstat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "lstat", Path: name, Err: fs.ErrInvalid}
	}
	info, err := os.Lstat(filepath.Join(filepath.FromSlash(f.dir), filepath.FromSlash(name)))
	if err != nil {
		return nil, &fs.PathError{Op: "lstat", Path: name, Err: err}
	}
	return info, nil
}

func New(fsys fs.FS, dir string) LstatFS {
	return &lstatFS{
		fsys: fsys,
		dir:  dir,
	}
}
