package jsonnetengine

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"

	"github.com/google/go-jsonnet"
	"github.com/google/go-jsonnet/ast"
	"github.com/mitchellh/mapstructure"
	"xorkevin.dev/anvil/confengine"
	"xorkevin.dev/anvil/util/kjson"
	"xorkevin.dev/kerrors"
)

type (
	// Engine is a jsonnet config engine
	Engine struct {
		vm          *jsonnet.VM
		args        map[string]any
		strout      bool
		libname     string
		nativeFuncs []NativeFunc
	}

	// NativeFunc is a jsonnet function implemented in go
	NativeFunc struct {
		name   string
		fn     func(args []any) (any, error)
		params []string
	}

	// Opt are jsonnet engine constructor options
	Opt = func(e *Engine)
)

// New creates a new [*Engine] which is rooted at a particular file system
func New(fsys fs.FS, opts ...Opt) *Engine {
	eng := &Engine{
		vm:          jsonnet.MakeVM(),
		strout:      false,
		libname:     "anvil:std",
		nativeFuncs: nil,
	}
	for _, i := range opts {
		i(eng)
	}

	eng.vm.SetTraceOut(io.Discard)
	eng.vm.StringOutput = eng.strout

	var stdlib strings.Builder
	stdlib.WriteString("{\n")

	for _, v := range append([]NativeFunc{
		{
			name:   "getargs",
			fn:     eng.getargs,
			params: []string{},
		},
		{
			name: "jsonMarshal",
			fn: func(args []any) (any, error) {
				if len(args) != 1 {
					return nil, kerrors.WithKind(nil, confengine.ErrInvalidArgs, "jsonMarshal needs 1 argument")
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
					return nil, kerrors.WithKind(nil, confengine.ErrInvalidArgs, "jsonMergePatch needs 2 arguments")
				}
				return kjson.MergePatch(args[0], args[1]), nil
			},
			params: []string{"a", "b"},
		},
		{
			name: "pathJoin",
			fn: func(args []any) (any, error) {
				if len(args) != 1 {
					return nil, kerrors.WithKind(nil, confengine.ErrInvalidArgs, "pathJoin needs 1 argument")
				}
				var segments []string
				if err := mapstructure.Decode(args[0], &segments); err != nil {
					return nil, kerrors.WithKind(err, confengine.ErrInvalidArgs, "Failed to decode path segments")
				}
				return path.Join(segments...), nil
			},
			params: []string{"v"},
		},
	}, eng.nativeFuncs...) {
		params := make(ast.Identifiers, 0, len(v.params))
		for _, i := range v.params {
			params = append(params, ast.Identifier(i))
		}
		eng.vm.NativeFunction(&jsonnet.NativeFunction{
			Name:   v.name,
			Func:   v.fn,
			Params: params,
		})
		paramstr := ""
		if len(v.params) > 0 {
			paramstr = strings.Join(v.params, ", ")
		}
		stdlib.WriteString(v.name)
		stdlib.WriteString("(")
		stdlib.WriteString(paramstr)
		stdlib.WriteString(`):: std.native("`)
		stdlib.WriteString(v.name)
		stdlib.WriteString(`")(`)
		stdlib.WriteString(paramstr)
		stdlib.WriteString("),\n")
	}
	stdlib.WriteString("}\n")
	eng.vm.Importer(newFSImporter(fsys, eng.libname, stdlib.String()))
	return eng
}

func OptStrOut(strout bool) Opt {
	return func(e *Engine) {
		e.strout = strout
	}
}

func OptLibName(name string) Opt {
	return func(e *Engine) {
		e.libname = name
	}
}

func OptNativeFuncs(fns []NativeFunc) Opt {
	return func(e *Engine) {
		e.nativeFuncs = fns
	}
}

type (
	Builder []Opt
)

func (b Builder) Build(fsys fs.FS) (confengine.ConfEngine, error) {
	return New(fsys, b...), nil
}

func (e *Engine) getargs(args []any) (any, error) {
	if len(args) != 0 {
		return nil, kerrors.WithKind(nil, confengine.ErrInvalidArgs, "getargs does not take arguments")
	}
	return e.args, nil
}

// Exec implements [confengine.ConfEngine] and generates config using jsonnet
func (e *Engine) Exec(ctx context.Context, name string, args map[string]any, stdout io.Writer) ([]byte, error) {
	// reset the value cache by resetting the external vars
	e.vm.ExtReset()

	if stdout == nil {
		stdout = io.Discard
	}
	e.vm.SetTraceOut(stdout)
	if args == nil {
		args = map[string]any{}
	}
	e.args = args
	b, err := e.vm.EvaluateFile(name)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate jsonnet")
	}
	return []byte(b), nil
}

type (
	fsImporter struct {
		root          fs.FS
		contentsCache map[string]*fsContents
		libname       string
		stl           jsonnet.Contents
	}

	fsContents struct {
		contents jsonnet.Contents
		err      error
	}
)

func newFSImporter(root fs.FS, libname string, stl string) *fsImporter {
	return &fsImporter{
		root:          root,
		contentsCache: map[string]*fsContents{},
		libname:       libname,
		stl:           jsonnet.MakeContents(stl),
	}
}

func (f *fsImporter) importFile(fspath string) (jsonnet.Contents, error) {
	if c, ok := f.contentsCache[fspath]; ok {
		return c.contents, c.err
	}
	var c jsonnet.Contents
	b, err := fs.ReadFile(f.root, fspath)
	if err == nil {
		c = jsonnet.MakeContentsRaw(b)
	}
	f.contentsCache[fspath] = &fsContents{
		contents: c,
		err:      err,
	}
	return c, err
}

// Import implements [github.com/google/go-jsonnet.Importer]
func (f *fsImporter) Import(importedFrom, importedPath string) (jsonnet.Contents, string, error) {
	if importedPath == f.libname {
		return f.stl, f.libname, nil
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
		return jsonnet.Contents{}, "", kerrors.WithMsg(fs.ErrInvalid, fmt.Sprintf("Invalid filepath %s from %s", importedPath, importedFrom))
	}
	c, err := f.importFile(fspath)
	if err != nil {
		return jsonnet.Contents{}, "", kerrors.WithMsg(err, fmt.Sprintf("Failed to read file: %s", fspath))
	}
	return c, fspath, err
}
