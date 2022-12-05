package jsonnetengine

import (
	"io/fs"
	"path"

	"github.com/google/go-jsonnet"
	"github.com/google/go-jsonnet/ast"
	"xorkevin.dev/anvil/confengine"
	"xorkevin.dev/kerrors"
)

type (
	// Engine is a jsonnet config engine
	Engine struct {
		vm *jsonnet.VM
	}
)

// New creates a new [*Engine] which is rooted at a particular file system
func New(fsys fs.FS) *Engine {
	vm := jsonnet.MakeVM()
	vm.Importer(newFSImporter(fsys))
	return &Engine{
		vm: vm,
	}
}

// Exec implements [xorkevin.dev/anvil/confengine.ConfEngine] and generates config using jsonnet
func (e *Engine) Exec(name string, vars confengine.Vars) ([]byte, error) {
	e.vm.ExtReset()
	for k, v := range vars {
		e.vm.ExtCode(k, string(v))
	}
	for k, v := range (confengine.Functions{}) {
		params := make(ast.Identifiers, 0, len(v.Params))
		for _, i := range v.Params {
			params = append(params, ast.Identifier(i))
		}
		e.vm.NativeFunction(&jsonnet.NativeFunction{
			Name:   k,
			Func:   v.Function,
			Params: params,
		})
	}
	b, err := e.vm.EvaluateFile(name)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate jsonnet")
	}
	return []byte(b), nil
}

type (
	fsImporter struct {
		root     fs.FS
		contents map[string]jsonnet.Contents
	}
)

func newFSImporter(root fs.FS) *fsImporter {
	return &fsImporter{
		root:     root,
		contents: map[string]jsonnet.Contents{},
	}
}

func (f *fsImporter) importFile(fspath string) (jsonnet.Contents, error) {
	if c, ok := f.contents[fspath]; ok {
		return c, nil
	}
	b, err := fs.ReadFile(f.root, fspath)
	if err != nil {
		return jsonnet.Contents{}, kerrors.WithMsg(err, "Failed to read file")
	}
	c := jsonnet.MakeContentsRaw(b)
	f.contents[fspath] = c
	return c, nil
}

// Import implements [github.com/google/go-jsonnet.Importer]
func (f *fsImporter) Import(importedFrom, importedPath string) (jsonnet.Contents, string, error) {
	var fspath string
	if path.IsAbs(importedPath) {
		// make absolute paths relative to the root fs
		fspath = path.Clean(importedPath[1:])
	} else {
		fspath = path.Join(importedFrom, importedPath)
	}
	c, err := f.importFile(fspath)
	if err != nil {
		return jsonnet.Contents{}, "", err
	}
	return c, fspath, err
}
