package jsonnetengine

import (
	"io/fs"
	"path"
	"strings"

	"github.com/google/go-jsonnet"
	"github.com/google/go-jsonnet/ast"
	"xorkevin.dev/anvil/confengine"
	"xorkevin.dev/anvil/util/kjson"
	"xorkevin.dev/kerrors"
)

type (
	// Engine is a jsonnet config engine
	Engine struct {
		vm   *jsonnet.VM
		args map[string]any
	}

	NativeFunc struct {
		name   string
		fn     func(args []any) (any, error)
		params []string
	}
)

// New creates a new [*Engine] which is rooted at a particular file system
func New(fsys fs.FS, stdname string, nativeFuncs []NativeFunc) *Engine {
	vm := jsonnet.MakeVM()
	eng := &Engine{
		vm: vm,
	}
	var stl strings.Builder
	stl.WriteString("{\n")

	for _, v := range append([]NativeFunc{
		{
			name:   "envArgs",
			fn:     eng.getEnvArgs,
			params: []string{},
		},
		{
			name: "jsonMarshal",
			fn: func(args []any) (any, error) {
				if len(args) != 1 {
					return nil, kerrors.WithKind(nil, confengine.ErrorInvalidArgs, "jsonMarshal needs 1 argument")
				}
				b, err := kjson.Marshal(args[0])
				if err != nil {
					return nil, kerrors.WithMsg(err, "Failed to marshal json")
				}
				return string(b), nil
			},
			params: []string{"v"},
		},
		{
			name: "jsonMergePatch",
			fn: func(args []any) (any, error) {
				if len(args) != 2 {
					return nil, kerrors.WithKind(nil, confengine.ErrorInvalidArgs, "jsonMergePatch needs 2 arguments")
				}
				return kjson.MergePatch(args[0], args[1]), nil
			},
			params: []string{"a", "b"},
		},
	}, nativeFuncs...) {
		params := make(ast.Identifiers, 0, len(v.params))
		for _, i := range v.params {
			params = append(params, ast.Identifier(i))
		}
		vm.NativeFunction(&jsonnet.NativeFunction{
			Name:   v.name,
			Func:   v.fn,
			Params: params,
		})
		paramstr := ""
		if len(v.params) > 0 {
			paramstr = strings.Join(v.params, ", ")
		}
		stl.WriteString(v.name)
		stl.WriteString("(")
		stl.WriteString(paramstr)
		stl.WriteString(`):: std.native("`)
		stl.WriteString(v.name)
		stl.WriteString(`")(`)
		stl.WriteString(paramstr)
		stl.WriteString("),\n")
	}
	stl.WriteString("}\n")
	stlstr := stl.String()
	vm.Importer(newFSImporter(fsys, stdname, stlstr))
	return eng
}

func (e *Engine) getEnvArgs(args []any) (any, error) {
	if len(args) != 0 {
		return nil, kerrors.WithKind(nil, confengine.ErrorInvalidArgs, "envArgs does not take arguments")
	}
	return e.args, nil
}

// Exec implements [confengine.ConfEngine] and generates config using jsonnet
func (e *Engine) Exec(name string, args map[string]any) ([]byte, error) {
	e.args = args
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
		stdname  string
		stl      jsonnet.Contents
	}
)

func newFSImporter(root fs.FS, stdname string, stl string) *fsImporter {
	return &fsImporter{
		root:     root,
		contents: map[string]jsonnet.Contents{},
		stdname:  stdname,
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
	if importedPath == f.stdname {
		return f.stl, f.stdname, nil
	}

	var fspath string
	if path.IsAbs(importedPath) {
		// make absolute paths relative to the root fs
		fspath = path.Clean(importedPath[1:])
	} else {
		// paths are otherwise relative to the file importing them
		fspath = path.Join(path.Dir(importedFrom), importedPath)
	}
	if !fs.ValidPath(fspath) {
		return jsonnet.Contents{}, "", kerrors.WithMsg(fs.ErrInvalid, "Invalid filepath")
	}
	c, err := f.importFile(fspath)
	if err != nil {
		return jsonnet.Contents{}, "", err
	}
	return c, fspath, err
}
