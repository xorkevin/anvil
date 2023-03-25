package maskfs

import (
	"errors"
	"io/fs"
	"path"

	"xorkevin.dev/anvil/util/lstatfs"
)

var ErrTargetMasked = errors.New("target masked")

type (
	FileFilter = func(p string, entry fs.DirEntry) (bool, error)

	maskFS struct {
		fsys   fs.FS
		dir    string
		filter FileFilter
	}
)

func (f *maskFS) checkFile(name string) error {
	info, err := lstatfs.Lstat(f.fsys, name)
	if err != nil {
		return err
	}
	ok, err := f.filter(path.Join(f.dir, name), fs.FileInfoToDirEntry(info))
	if err != nil {
		return err
	}
	if !ok {
		return &fs.PathError{Op: "open", Path: name, Err: ErrTargetMasked}
	}
	return nil
}

func (f *maskFS) Open(name string) (fs.File, error) {
	if err := f.checkFile(name); err != nil {
		return nil, err
	}
	return f.fsys.Open(name)
}

func (f *maskFS) Stat(name string) (fs.FileInfo, error) {
	if err := f.checkFile(name); err != nil {
		return nil, err
	}
	return fs.Stat(f.fsys, name)
}

func (f *maskFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if err := f.checkFile(name); err != nil {
		return nil, err
	}
	entries, err := fs.ReadDir(f.fsys, name)
	if err != nil {
		return nil, err
	}
	res := make([]fs.DirEntry, 0, len(entries))
	for _, i := range entries {
		if ok, err := f.filter(path.Join(f.dir, name, i.Name()), i); err != nil {
			return nil, err
		} else if !ok {
			continue
		}
		res = append(res, i)
	}
	return res, nil
}

func (f *maskFS) ReadFile(name string) ([]byte, error) {
	if err := f.checkFile(name); err != nil {
		return nil, err
	}
	return fs.ReadFile(f.fsys, name)
}

func (f *maskFS) Glob(pattern string) ([]string, error) {
	return fs.Glob(f.fsys, pattern)
}

func (f *maskFS) Sub(dir string) (fs.FS, error) {
	fsys, err := fs.Sub(f.fsys, dir)
	if err != nil {
		return nil, err
	}
	return New(fsys, path.Join(f.dir, dir), f.filter), nil
}

func New(fsys fs.FS, dir string, filter FileFilter) fs.FS {
	return &maskFS{
		fsys:   fsys,
		dir:    dir,
		filter: filter,
	}
}
