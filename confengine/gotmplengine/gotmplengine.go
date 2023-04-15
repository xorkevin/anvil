package gotmplengine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"text/template"

	"xorkevin.dev/anvil/confengine"
	"xorkevin.dev/kerrors"
)

type (
	// Engine is a go template config engine
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

// Exec implements [confengine.ConfEngine] and generates configs with go template
func (e *Engine) Exec(ctx context.Context, name string, args map[string]any, stdout io.Writer) (io.ReadCloser, error) {
	t, err := template.ParseFS(e.fsys, name)
	if err != nil {
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed parsing go templates: %s", name))
	}
	var b bytes.Buffer
	if err := t.Execute(&b, args); err != nil {
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed executing go template: %s", t.Name()))
	}
	return io.NopCloser(&b), nil
}
