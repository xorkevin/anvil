package scriptengine

import (
	"context"
	"fmt"
	"io"
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
	// ScriptEngine is a script engine
	ScriptEngine interface {
		Exec(ctx context.Context, name string, fn string, args map[string]any, stdout io.Writer) (any, error)
	}

	// Builder builds a [ScriptEngine]
	Builder interface {
		Build(fsys fs.FS) (ScriptEngine, error)
	}

	// BuilderFunc implements Builder for a function
	BuilderFunc func(fsys fs.FS) (ScriptEngine, error)

	// Map is a map from kinds to [Builder]
	Map map[string]Builder
)

func (f BuilderFunc) Build(fsys fs.FS) (ScriptEngine, error) {
	return f(fsys)
}

// Build builds a [ScriptEngine] for a known kind
func (m Map) Build(kind string, fsys fs.FS) (ScriptEngine, error) {
	a, ok := m[kind]
	if !ok {
		return nil, kerrors.WithKind(nil, ErrNotSupported, fmt.Sprintf("Engine kind not supported: %s", kind))
	}
	eng, err := a.Build(fsys)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to build script engine")
	}
	return eng, nil
}
