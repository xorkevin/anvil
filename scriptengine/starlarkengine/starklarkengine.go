package starlarkengine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"

	"go.starlark.net/starlark"
	"xorkevin.dev/anvil/util/stackset"
	"xorkevin.dev/kerrors"
)

type (
	Engine struct {
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
	return &Engine{
		modLoader: &modLoader{
			root:     fsys,
			modCache: map[string]*loadedModule{},
			set:      stackset.New[string](),
		},
	}
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
			// TODO: add predeclared globals
			vals, err = starlark.ExecFile(&starlark.Thread{
				Name:  module,
				Print: discardPrinter,
				Load:  fromLoader{l: l, from: module}.load,
			}, module, b, nil)
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
	var fspath string
	if path.IsAbs(module) {
		fspath = path.Clean(module[1:])
	} else {
		fspath = path.Join(path.Dir(from), module)
	}
	if !fs.ValidPath(fspath) {
		return nil, kerrors.WithMsg(fs.ErrInvalid, fmt.Sprintf("Invalid filepath %s from %s", module, from))
	}
	vals, err := l.loadFile(fspath)
	if err != nil {
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed to read module: %s", fspath))
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

func (e *Engine) Exec(ctx context.Context, module string, fn string, args map[string]any, stdout io.Writer) (map[string]any, error) {
	if stdout == nil {
		stdout = io.Discard
	}
	vals, err := e.modLoader.load("", module)
	if err != nil {
		return nil, err
	}
	f, ok := vals[fn]
	if !ok {
		return nil, kerrors.WithMsg(nil, fmt.Sprintf("Global %s not defined for module %s", fn, module))
	}
	// TODO: check f.Type()
	// TODO: pass arguments
	_, err = starlark.Call(&starlark.Thread{
		Name:  module + "." + fn,
		Print: writerPrinter{w: stdout}.print,
		Load:  errLoader,
	}, f, nil, nil)
	if err != nil {
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed executing function %s in module %s", fn, module))
	}
	// TODO: convert value to any
	return nil, nil
}

func starlarkToGoValue(x starlark.Value, ss *stackset.Any) (_ any, retErr error) {
	switch x.(type) {
	case starlark.IterableMapping, starlark.Iterable, starlark.HasAttrs:
		if !ss.Push(x) {
			return nil, kerrors.WithMsg(nil, "Cycle in starlark value")
		}
		defer func() {
			if v, ok := ss.Pop(); !ok {
				retErr = errors.Join(retErr, kerrors.WithMsg(nil, "Failed checking starlark value cycle due to missing element"))
			} else if v != x {
				retErr = errors.Join(retErr, kerrors.WithMsg(nil, "Failed checking starlark value cycle due to mismatched element"))
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
				return nil, kerrors.WithMsg(nil, "Int out of range")
			}
			return int(i), nil
		}

	case starlark.Float:
		return float64(x), nil

	case starlark.String:
		return string(x), nil

	case starlark.IterableMapping:
		{
			v := map[string]any{}
			for _, i := range x.Items() {
				k, ok := i[0].(starlark.String)
				if !ok {
					return nil, kerrors.WithMsg(nil, "Non-string key in map")
				}
				vv, err := starlarkToGoValue(i[1], ss)
				if err != nil {
					return nil, err
				}
				v[string(k)] = vv
			}
			return v, nil
		}

	case starlark.Iterable:
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

	case starlark.HasAttrs:
		{
			v := map[string]any{}
			for _, k := range x.AttrNames() {
				a, err := x.Attr(k)
				if err != nil {
					return nil, kerrors.WithMsg(err, "Failed retrieving struct attr")
				}
				vv, err := starlarkToGoValue(a, ss)
				if err != nil {
					return nil, err
				}
				v[k] = vv
			}
			return v, nil
		}

	default:
		return nil, kerrors.WithMsg(nil, "Unknown starlark type")
	}
}

func goToStarlarkValue(x any, ss *stackset.Any) (_ starlark.Value, retErr error) {
	if x == nil {
		return starlark.None, nil
	}
	switch x.(type) {
	case map[string]any, []any:
		if !ss.Push(x) {
			return nil, kerrors.WithMsg(nil, "Cycle in go value")
		}
		defer func() {
			if v, ok := ss.Pop(); !ok {
				retErr = errors.Join(retErr, kerrors.WithMsg(nil, "Failed checking go value cycle due to missing element"))
			} else if v != x {
				retErr = errors.Join(retErr, kerrors.WithMsg(nil, "Failed checking go value cycle due to mismatched element"))
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
		return nil, kerrors.WithMsg(nil, "Unsupported go type")
	}
}
