package confengine

import (
	"fmt"
	"io/fs"

	"xorkevin.dev/kerrors"
)

var (
	// ErrNotSupported is returned when the kind is not supported
	ErrNotSupported errNotSupported
	// ErrInvalidArgs is returned when calling an engine native function with invalid args
	ErrInvalidArgs errInvalidArgs
)

type (
	errNotSupported struct{}
	errInvalidArgs  struct{}
)

func (e errNotSupported) Error() string {
	return "Engine kind not supported"
}

func (e errInvalidArgs) Error() string {
	return "Invalid args"
}

type (
	// ConfEngine is a config engine
	ConfEngine interface {
		Exec(name string, args map[string]any) ([]byte, error)
	}

	// Builder builds a [ConfEngine]
	Builder interface {
		Build(fsys fs.FS) (ConfEngine, error)
	}

	// BuilderFunc implements Builder for a function
	BuilderFunc func(fsys fs.FS) (ConfEngine, error)

	// Map is a map from kinds to [Builder]
	Map map[string]Builder
)

func (f BuilderFunc) Build(fsys fs.FS) (ConfEngine, error) {
	return f(fsys)
}

// Build builds a [ConfEngine] for a known kind
func (m Map) Build(kind string, fsys fs.FS) (ConfEngine, error) {
	a, ok := m[kind]
	if !ok {
		return nil, kerrors.WithKind(nil, ErrNotSupported, fmt.Sprintf("Engine kind not supported: %s", kind))
	}
	eng, err := a.Build(fsys)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to build config engine")
	}
	return eng, nil
}
