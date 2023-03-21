package readlinkfs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

var (
	ErrNotImplemented  = errors.New("not implemented")
	ErrTargetOutsideFS = errors.New("target outside fs")
)

type (
	ReadLinkFS interface {
		fs.FS
		// ReadLink returns the destination of the named symbolic link.
		// Link destinations will always be slash-separated paths relative to
		// the link's directory. The link destination is guaranteed to be
		// a path inside FS.
		ReadLink(name string) (string, error)
	}
)

// ReadLink returns the destination of the named symbolic link.
//
// If fsys does not implement ReadLinkFS, then ReadLink returns an error.
func ReadLink(fsys fs.FS, name string) (string, error) {
	rl, ok := fsys.(ReadLinkFS)
	if !ok {
		return "", &fs.PathError{Op: "readlink", Path: name, Err: ErrNotImplemented}
	}
	return rl.ReadLink(name)
}

type (
	readlinkFS struct {
		fsys fs.FS
		dir  string
	}
)

func (f *readlinkFS) Open(name string) (fs.File, error) {
	return f.fsys.Open(name)
}

func (f *readlinkFS) Stat(name string) (fs.FileInfo, error) {
	return fs.Stat(f.fsys, name)
}

func (f *readlinkFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(f.fsys, name)
}

func (f *readlinkFS) ReadFile(name string) ([]byte, error) {
	return fs.ReadFile(f.fsys, name)
}

func (f *readlinkFS) Glob(pattern string) ([]string, error) {
	return fs.Glob(f.fsys, pattern)
}

func (f *readlinkFS) Sub(dir string) (fs.FS, error) {
	return fs.Sub(f.fsys, dir)
}

func (f *readlinkFS) ReadLink(name string) (string, error) {
	if !fs.ValidPath(name) {
		return "", &fs.PathError{Op: "readlink", Path: name, Err: fs.ErrInvalid}
	}
	target, err := os.Readlink(filepath.Join(filepath.FromSlash(f.dir), filepath.FromSlash(name)))
	if err != nil {
		return "", err
	}
	target = filepath.ToSlash(target)
	if path.IsAbs(target) {
		return "", &fs.PathError{Op: "readlink", Path: name, Err: fmt.Errorf("%w: target %s is absolute", ErrTargetOutsideFS, target)}
	}
	if !fs.ValidPath(path.Join(path.Dir(name), target)) {
		return "", &fs.PathError{Op: "readlink", Path: name, Err: fmt.Errorf("%w: target %s is outside the file system", ErrTargetOutsideFS, target)}
	}
	return target, nil
}

func New(fsys fs.FS, dir string) ReadLinkFS {
	return &readlinkFS{
		fsys: fsys,
		dir:  dir,
	}
}
