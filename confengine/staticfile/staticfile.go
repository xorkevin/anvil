package staticfile

import (
	"context"
	"fmt"
	"io"
	"io/fs"

	"xorkevin.dev/anvil/confengine"
	"xorkevin.dev/kerrors"
)

type (
	// Engine is a static file config engine
	Engine struct {
		fsys fs.FS
	}
)

func New(fsys fs.FS) *Engine {
	return &Engine{
		fsys: fsys,
	}
}

type (
	Builder struct{}
)

func (b Builder) Build(fsys fs.FS) (confengine.ConfEngine, error) {
	return New(fsys), nil
}

// Exec implements [confengine.ConfEngine] and copies static file configs
func (e *Engine) Exec(ctx context.Context, name string, args map[string]any, stdout io.Writer) ([]byte, error) {
	b, err := fs.ReadFile(e.fsys, name)
	if err != nil {
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed to read file: %s", name))
	}
	return b, nil
}
