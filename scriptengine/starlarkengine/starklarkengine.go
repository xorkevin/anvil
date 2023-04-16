package starlarkengine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"reflect"
	"strings"

	starjson "go.starlark.net/lib/json"
	starmath "go.starlark.net/lib/math"
	startime "go.starlark.net/lib/time"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"xorkevin.dev/anvil/scriptengine"
	"xorkevin.dev/anvil/util/stackset"
	"xorkevin.dev/kerrors"
)

type (
	Engine struct {
		libname   string
		modLoader *modLoader
	}

	loadedModule struct {
		vals starlark.StringDict
		err  error
	}

	modLoader struct {
		root     fs.FS
		modCache map[string]*loadedModule
		set      *stackset.StackSet[string]
		universe map[string]starlark.StringDict
		globals  starlark.StringDict
	}

	fromLoader struct {
		l    *modLoader
		from string
	}

	writerPrinter struct {
		w io.Writer
	}
)

func New(fsys fs.FS) *Engine {
	eng := &Engine{
		libname: "anvil:std",
	}
	eng.modLoader = &modLoader{
		root:     fsys,
		modCache: map[string]*loadedModule{},
		set:      stackset.New[string](),
		universe: map[string]starlark.StringDict{
			eng.libname + ":json":   starjson.Module.Members,
			eng.libname + ":math":   starmath.Module.Members,
			eng.libname + ":time":   startime.Module.Members,
			eng.libname:             universeLibBase{}.mod(),
			eng.libname + ":crypto": universeLibCrypto{}.mod(),
			eng.libname + ":vault":  universeLibVault{}.mod(),
		},
		globals: starlark.StringDict{
			"struct": starlark.NewBuiltin("struct", starlarkstruct.Make),
			"module": starlark.NewBuiltin("module", starlarkstruct.MakeModule),
		},
	}
	return eng
}

type (
	Builder struct{}
)

func (b Builder) Build(fsys fs.FS) (scriptengine.ScriptEngine, error) {
	return New(fsys), nil
}

func (w writerPrinter) print(t *starlark.Thread, msg string) {
	fmt.Fprintln(w.w, msg)
}

func discardPrinter(t *starlark.Thread, msg string) {}

// ErrImportCycle is returned when module dependencies form a cycle
var ErrImportCycle errImportCycle

type (
	errImportCycle struct{}
)

func (e errImportCycle) Error() string {
	return "Import cycle"
}

func (l *modLoader) loadFile(module string) (starlark.StringDict, error) {
	if m, ok := l.modCache[module]; ok {
		return m.vals, m.err
	}
	var vals starlark.StringDict
	b, err := fs.ReadFile(l.root, module)
	if err == nil {
		if !l.set.Push(module) {
			err = fmt.Errorf("%w: Import cycle on module: %s -> %s", ErrImportCycle, strings.Join(l.set.Slice(), ","), module)
		} else {
			vals, err = starlark.ExecFile(&starlark.Thread{
				Name:  module,
				Print: discardPrinter,
				Load:  fromLoader{l: l, from: module}.load,
			}, module, b, l.globals)
			v, ok := l.set.Pop()
			if !ok {
				err = errors.Join(err, fmt.Errorf("%w: Failed checking import cycle due to missing element on module %s", ErrImportCycle, module))
			} else if v != module {
				err = errors.Join(err, fmt.Errorf("%w: Failed checking import cycle due to mismatched element on module %s, %s; %s", ErrImportCycle, module, v, strings.Join(l.set.Slice(), ",")))
			}
			if err != nil {
				vals = nil
			}
		}
	}
	l.modCache[module] = &loadedModule{
		vals: vals,
		err:  err,
	}
	return vals, err
}

func (l *modLoader) load(from, module string) (starlark.StringDict, error) {
	if m, ok := l.universe[module]; ok {
		return m, nil
	}

	var name string
	if path.IsAbs(module) {
		name = path.Clean(module[1:])
	} else {
		name = path.Join(path.Dir(from), module)
	}
	if !fs.ValidPath(name) {
		return nil, fmt.Errorf("%w: Invalid filepath %s from %s", fs.ErrInvalid, module, from)
	}
	vals, err := l.loadFile(name)
	if err != nil {
		return nil, fmt.Errorf("Failed to read module %s: %w", name, err)
	}
	return vals, nil
}

func (l fromLoader) load(t *starlark.Thread, module string) (starlark.StringDict, error) {
	return l.l.load(l.from, module)
}

// ErrNoRuntimeLoad is returned when attempting to load modules not ata the top level
var ErrNoRuntimeLoad errNoRuntimeLoad

type (
	errNoRuntimeLoad struct{}
)

func (e errNoRuntimeLoad) Error() string {
	return "May not load modules not at the top level"
}

func errLoader(t *starlark.Thread, module string) (starlark.StringDict, error) {
	return nil, ErrNoRuntimeLoad
}

func (e *Engine) Exec(ctx context.Context, name string, fn string, args map[string]any, stdout io.Writer) (any, error) {
	vals, err := e.modLoader.load("", name)
	if err != nil {
		return nil, err
	}
	f, ok := vals[fn]
	if !ok {
		return nil, kerrors.WithMsg(nil, fmt.Sprintf("Global %s not defined for module %s", fn, name))
	}
	if _, ok := f.(starlark.Callable); !ok {
		return nil, kerrors.WithMsg(nil, fmt.Sprintf("Global %s in module %s is not callable", fn, name))
	}
	ss := stackset.NewAny()
	sargs, err := goToStarlarkValue(args, ss)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed converting go value args to starlark values")
	}
	if stdout == nil {
		stdout = io.Discard
	}
	sv, err := starlark.Call(&starlark.Thread{
		Name:  name + "." + fn,
		Print: writerPrinter{w: stdout}.print,
		Load:  errLoader,
	}, f, []starlark.Value{sargs}, nil)
	if err != nil {
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed executing function %s in module %s", fn, name))
	}
	v, err := starlarkToGoValue(sv, ss)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed converting starlark returned values to go values")
	}
	return v, nil
}

func starlarkToGoValue(x starlark.Value, ss *stackset.Any) (_ any, retErr error) {
	switch x.(type) {
	case *starlark.Dict, *starlark.List:
		if !ss.Push(x) {
			return nil, errors.New("Cycle in starlark value")
		}
		defer func() {
			if v, ok := ss.Pop(); !ok {
				retErr = errors.Join(retErr, errors.New("Failed checking starlark value cycle due to missing element"))
			} else if v != x {
				retErr = errors.Join(retErr, errors.New("Failed checking starlark value cycle due to mismatched element"))
			}
		}()
	}

	switch x := x.(type) {
	case starlark.NoneType:
		return nil, nil

	case starlark.Bool:
		return bool(x), nil

	case starlark.Int:
		{
			i, ok := x.Int64()
			if !ok {
				return nil, errors.New("Int out of range")
			}
			return int(i), nil
		}

	case starlark.Float:
		return float64(x), nil

	case starlark.String:
		return string(x), nil

	case *starlark.Dict:
		{
			v := map[string]any{}
			for _, i := range x.Items() {
				k, ok := i[0].(starlark.String)
				if !ok {
					return nil, errors.New("Non-string key in map")
				}
				vv, err := starlarkToGoValue(i[1], ss)
				if err != nil {
					return nil, err
				}
				v[string(k)] = vv
			}
			return v, nil
		}

	case *starlark.List:
		{
			var v []any
			iter := x.Iterate()
			defer iter.Done()
			var elem starlark.Value
			for iter.Next(&elem) {
				vv, err := starlarkToGoValue(elem, ss)
				if err != nil {
					return nil, err
				}
				v = append(v, vv)
			}
			return v, nil
		}

	default:
		return nil, errors.New("Unknown starlark type")
	}
}

func goToStarlarkValue(x any, ss *stackset.Any) (_ starlark.Value, retErr error) {
	if x == nil {
		return starlark.None, nil
	}
	switch x.(type) {
	case map[string]any, []any:
		ptr := reflect.ValueOf(x).UnsafePointer()
		if !ss.Push(ptr) {
			return nil, errors.New("Cycle in go value")
		}
		defer func() {
			if v, ok := ss.Pop(); !ok {
				retErr = errors.Join(retErr, errors.New("Failed checking go value cycle due to missing element"))
			} else if v != ptr {
				retErr = errors.Join(retErr, errors.New("Failed checking go value cycle due to mismatched element"))
			}
		}()
	}
	switch x := x.(type) {
	case bool:
		return starlark.Bool(x), nil
	case int:
		return starlark.MakeInt(x), nil
	case int8:
		return starlark.MakeInt(int(x)), nil
	case int16:
		return starlark.MakeInt(int(x)), nil
	case int32:
		return starlark.MakeInt(int(x)), nil
	case int64:
		return starlark.MakeInt64(x), nil
	case uint:
		return starlark.MakeUint(x), nil
	case uint8:
		return starlark.MakeUint(uint(x)), nil
	case uint16:
		return starlark.MakeUint(uint(x)), nil
	case uint32:
		return starlark.MakeUint(uint(x)), nil
	case uint64:
		return starlark.MakeUint64(x), nil
	case uintptr:
		return starlark.MakeUint(uint(x)), nil
	case float32:
		return starlark.Float(x), nil
	case float64:
		return starlark.Float(x), nil
	case string:
		return starlark.String(x), nil
	case map[string]any:
		{
			d := starlark.NewDict(len(x))
			for k, v := range x {
				vv, err := goToStarlarkValue(v, ss)
				if err != nil {
					return nil, err
				}
				d.SetKey(starlark.String(k), vv)
			}
			return d, nil
		}
	case []any:
		{
			l := make([]starlark.Value, 0, len(x))
			for _, i := range x {
				vv, err := goToStarlarkValue(i, ss)
				if err != nil {
					return nil, err
				}
				l = append(l, vv)
			}
			return starlark.NewList(l), nil
		}
	default:
		return nil, errors.New("Unsupported go type")
	}
}
