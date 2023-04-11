package starlarkengine

import (
	"io/fs"

	"go.starlark.net/starlark"
)

type (
	Engine struct {
		fsys  fs.FS
		cache map[string]*starlark.Program
	}

	modLoader struct {
		engine *Engine
		dir    string
	}
)
