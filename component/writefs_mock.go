package component

import (
	"bytes"
	"io"
	"io/fs"
)

type (
	WriteFSMock struct {
		Files map[string]string
	}

	WriteFSFileMock struct {
		name string
		b    *bytes.Buffer
		f    *WriteFSMock
	}
)

func NewWriteFSMock() *WriteFSMock {
	return &WriteFSMock{
		Files: map[string]string{},
	}
}

func (f *WriteFSMock) OpenFile(name string, flag int, mode fs.FileMode) (io.WriteCloser, error) {
	return NewWriteFSFileMock(name, f), nil
}

func NewWriteFSFileMock(name string, f *WriteFSMock) *WriteFSFileMock {
	return &WriteFSFileMock{
		name: name,
		b:    &bytes.Buffer{},
		f:    f,
	}
}

func (w *WriteFSFileMock) Write(p []byte) (n int, err error) {
	return w.b.Write(p)
}

func (w *WriteFSFileMock) Close() error {
	w.f.Files[w.name] = w.b.String()
	return nil
}
