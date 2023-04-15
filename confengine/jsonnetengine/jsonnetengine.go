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
		fsys        fs.FS
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
		fsys:        fsys,
		strout:      false,
		libname:     "anvil:std",
		nativeFuncs: nil,
	}
	for _, i := range opts {
		i(eng)
	}
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

type (
	confArgs struct {
		args map[string]any
	}
)

func (a confArgs) getargs(args []any) (any, error) {
	if len(args) != 0 {
		return nil, kerrors.WithKind(nil, confengine.ErrInvalidArgs, "getargs does not take arguments")
	}
	return a.args, nil
}

func (e *Engine) buildVM(args map[string]any, stdout io.Writer) *jsonnet.VM {
	if args == nil {
		args = map[string]any{}
	}
	if stdout == nil {
		stdout = io.Discard
	}

	vm := jsonnet.MakeVM()
	vm.SetTraceOut(stdout)
	vm.StringOutput = e.strout

	var stdlib strings.Builder
	stdlib.WriteString("{\n")

	for _, v := range append([]NativeFunc{
		{
			name:   "getargs",
			fn:     confArgs{args: args}.getargs,
			params: []string{},
		},
		{
			name: "jsonMarshal",
			fn: func(args []any) (any, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("%w: jsonMarshal needs 1 argument", confengine.ErrInvalidArgs)
				}
				b, err := kjson.Marshal(args[0])
				if err != nil {
					return nil, fmt.Errorf("Failed to marshal json: %w", err)
				}
				return string(b), nil
			},
			params: []string{"v"},
		},
		{
			name: "jsonUnmarshal",
			fn: func(args []any) (any, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("%w: jsonUnmarshal needs 1 argument", confengine.ErrInvalidArgs)
				}
				a, ok := args[0].(string)
				if !ok {
					return nil, fmt.Errorf("%w: Failed to decode arg as string", confengine.ErrInvalidArgs)
				}
				var v any
				if err := kjson.Unmarshal([]byte(a), &v); err != nil {
					return nil, fmt.Errorf("Failed to unmarshal json: %w", err)
				}
				return v, nil
			},
			params: []string{"v"},
		},
		{
			name: "jsonMergePatch",
			fn: func(args []any) (any, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("%w: jsonMergePatch needs 2 arguments", confengine.ErrInvalidArgs)
				}
				return kjson.MergePatch(args[0], args[1]), nil
			},
			params: []string{"a", "b"},
		},
		{
			name: "pathJoin",
			fn: func(args []any) (any, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("%w: pathJoin needs 1 argument", confengine.ErrInvalidArgs)
				}
				var segments []string
				if err := mapstructure.Decode(args[0], &segments); err != nil {
					return nil, fmt.Errorf("%w: Failed to decode path segments: %w", confengine.ErrInvalidArgs, err)
				}
				return path.Join(segments...), nil
			},
			params: []string{"v"},
		},
	}, e.nativeFuncs...) {
		paramstr := ""
		var params ast.Identifiers
		if len(v.params) > 0 {
			paramstr = strings.Join(v.params, ", ")
			params = make(ast.Identifiers, 0, len(v.params))
			for _, i := range v.params {
				params = append(params, ast.Identifier(i))
			}
		}
		vm.NativeFunction(&jsonnet.NativeFunction{
			Name:   v.name,
			Func:   v.fn,
			Params: params,
		})
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
	vm.Importer(newFSImporter(e.fsys, e.libname, stdlib.String()))
	return vm
}

// Exec implements [confengine.ConfEngine] and generates config using jsonnet
func (e *Engine) Exec(ctx context.Context, name string, args map[string]any, stdout io.Writer) (io.ReadCloser, error) {
	vm := e.buildVM(args, stdout)
	b, err := vm.EvaluateFile(name)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to execute jsonnet")
	}
	return io.NopCloser(strings.NewReader(b)), nil
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

	var name string
	if path.IsAbs(importedPath) {
		// make absolute paths relative to the root fs
		name = path.Clean(importedPath[1:])
	} else {
		// paths are otherwise relative to the file importing them
		name = path.Join(path.Dir(importedFrom), importedPath)
	}
	if !fs.ValidPath(name) {
		return jsonnet.Contents{}, "", fmt.Errorf("%w: Invalid filepath %s from %s", fs.ErrInvalid, importedPath, importedFrom)
	}
	c, err := f.importFile(name)
	if err != nil {
		return jsonnet.Contents{}, "", fmt.Errorf("Failed to read file %s: %w", name, err)
	}
	return c, name, err
}
