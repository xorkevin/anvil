package jsonnetengine

import (
	"io/fs"
	"path"
	"strings"

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
func New(fsys fs.FS, stlname string) *Engine {
	vm := jsonnet.MakeVM()
	var stl strings.Builder
	stl.WriteString("{\n")
	for k, v := range confengine.DefaultFunctions {
		params := make(ast.Identifiers, 0, len(v.Params))
		for _, i := range v.Params {
			params = append(params, ast.Identifier(i))
		}
		vm.NativeFunction(&jsonnet.NativeFunction{
			Name:   k,
			Func:   v.Function,
			Params: params,
		})
		paramstr := ""
		if len(v.Params) > 0 {
			paramstr = strings.Join(v.Params, ", ")
		}
		stl.WriteString(k)
		stl.WriteString("(")
		stl.WriteString(paramstr)
		stl.WriteString(`):: std.native("`)
		stl.WriteString(k)
		stl.WriteString(`")(`)
		stl.WriteString(paramstr)
		stl.WriteString("),\n")
	}
	stl.WriteString("}\n")
	stlstr := stl.String()
	vm.Importer(newFSImporter(fsys, stlname, stlstr))
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
		stlname  string
		stl      jsonnet.Contents
	}
)

func newFSImporter(root fs.FS, stlname string, stl string) *fsImporter {
	return &fsImporter{
		root:     root,
		contents: map[string]jsonnet.Contents{},
		stlname:  stlname,
		stl:      jsonnet.MakeContents(stl),
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
	if importedPath == f.stlname {
		return f.stl, f.stlname, nil
	}

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
