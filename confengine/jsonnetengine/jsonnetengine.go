package jsonnetengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"

	"github.com/google/go-jsonnet"
	"github.com/google/go-jsonnet/ast"
	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v3"
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
		Name   string
		Fn     func(args []any) (any, error)
		Params []string
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
			Name:   "getargs",
			Fn:     confArgs{args: args}.getargs,
			Params: []string{},
		},
		{
			Name: "jsonMarshal",
			Fn: func(args []any) (any, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("%w: jsonMarshal needs 1 argument", confengine.ErrInvalidArgs)
				}
				b, err := kjson.Marshal(args[0])
				if err != nil {
					return nil, fmt.Errorf("Failed to marshal json: %w", err)
				}
				return string(b), nil
			},
			Params: []string{"v"},
		},
		{
			Name: "jsonUnmarshal",
			Fn: func(args []any) (any, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("%w: jsonUnmarshal needs 1 argument", confengine.ErrInvalidArgs)
				}
				a, ok := args[0].(string)
				if !ok {
					return nil, fmt.Errorf("%w: JSON must be a string", confengine.ErrInvalidArgs)
				}
				// jsonnet treats all numbers as float64, therefore no need to decode
				// as number. It also does not handle [json.Number].
				var v any
				if err := json.Unmarshal([]byte(a), &v); err != nil {
					return nil, fmt.Errorf("Failed to unmarshal json: %w", err)
				}
				return v, nil
			},
			Params: []string{"v"},
		},
		{
			Name: "jsonMergePatch",
			Fn: func(args []any) (any, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("%w: jsonMergePatch needs 2 arguments", confengine.ErrInvalidArgs)
				}
				return kjson.MergePatch(args[0], args[1]), nil
			},
			Params: []string{"a", "b"},
		},
		{
			Name: "yamlMarshal",
			Fn: func(args []any) (any, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("%w: yamlMarshal needs 1 argument", confengine.ErrInvalidArgs)
				}
				b, err := yaml.Marshal(args[0])
				if err != nil {
					return nil, fmt.Errorf("Failed to marshal yaml: %w", err)
				}
				return string(b), nil
			},
			Params: []string{"v"},
		},
		{
			Name: "yamlUnmarshal",
			Fn: func(args []any) (any, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("%w: yamlUnmarshal needs 1 argument", confengine.ErrInvalidArgs)
				}
				a, ok := args[0].(string)
				if !ok {
					return nil, fmt.Errorf("%w: YAML must be a string", confengine.ErrInvalidArgs)
				}
				var v any
				if err := yaml.Unmarshal([]byte(a), &v); err != nil {
					return nil, fmt.Errorf("Failed to unmarshal yaml: %w", err)
				}
				return v, nil
			},
			Params: []string{"v"},
		},
		{
			Name: "pathJoin",
			Fn: func(args []any) (any, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("%w: pathJoin needs 1 argument", confengine.ErrInvalidArgs)
				}
				var segments []string
				if err := mapstructure.Decode(args[0], &segments); err != nil {
					return nil, fmt.Errorf("%w: Path segments must be an array of strings: %w", confengine.ErrInvalidArgs, err)
				}
				return path.Join(segments...), nil
			},
			Params: []string{"v"},
		},
		{
			Name: "sha256hex",
			Fn: func(args []any) (any, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("%w: sha256hex needs 1 argument", confengine.ErrInvalidArgs)
				}
				data, ok := args[0].(string)
				if !ok {
					return nil, fmt.Errorf("%w: sha256hex must have string argument", confengine.ErrInvalidArgs)
				}
				h := sha256.Sum256([]byte(data))
				return hex.EncodeToString(h[:]), nil
			},
			Params: []string{"v"},
		},
	}, e.nativeFuncs...) {
		paramstr := ""
		var params ast.Identifiers
		if len(v.Params) > 0 {
			paramstr = strings.Join(v.Params, ", ")
			params = make(ast.Identifiers, 0, len(v.Params))
			for _, i := range v.Params {
				params = append(params, ast.Identifier(i))
			}
		}
		vm.NativeFunction(&jsonnet.NativeFunction{
			Name:   v.Name,
			Func:   v.Fn,
			Params: params,
		})
		stdlib.WriteString(v.Name)
		stdlib.WriteString("(")
		stdlib.WriteString(paramstr)
		stdlib.WriteString(`):: std.native("`)
		stdlib.WriteString(v.Name)
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
