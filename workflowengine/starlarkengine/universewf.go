package starlarkengine

import (
	"fmt"

	"go.starlark.net/starlark"
	"xorkevin.dev/anvil/util/stackset"
	"xorkevin.dev/anvil/workflowengine"
)

type (
	universeLibWF struct {
		events *workflowengine.EventHistory
	}

	eventActivityArgsKey struct{}

	eventActivityArgs struct {
		name   string
		args   []any
		kwargs map[string]any
	}

	eventActivityRetKey struct{}

	eventActivityRet struct {
		name string
		ret  any
	}
)

func (l universeLibWF) mod() starlark.StringDict {
	return starlark.StringDict{
		"execactivity": starlark.NewBuiltin("execactivity", l.execactivity),
	}
}

func (l *universeLibWF) serializeArgs(ss *stackset.Any, idx int, name string, args starlark.Tuple, kwargs []starlark.Tuple) (eventActivityArgs, error) {
	e := eventActivityArgs{
		name:   name,
		args:   make([]any, 0, len(args)),
		kwargs: make(map[string]any, len(kwargs)),
	}
	for n, i := range args {
		v, err := starlarkToGoValue(i, ss)
		if err != nil {
			return eventActivityArgs{}, fmt.Errorf("%w: Activity %s position argument %d not serializable at event log index %d: %w", workflowengine.ErrInvalidArgs, name, n, idx, err)
		}
		e.args = append(e.args, v)
	}
	for _, i := range kwargs {
		if len(i) != 2 {
			return eventActivityArgs{}, fmt.Errorf("%w: Activity %s malformed keyword argument at event log index %d", workflowengine.ErrInvalidArgs, name, idx)
		}
		key, ok := i[0].(starlark.String)
		if !ok {
			return eventActivityArgs{}, fmt.Errorf("%w: Activity %s malformed keyword argument at event log index %d", workflowengine.ErrInvalidArgs, name, idx)
		}
		v, err := starlarkToGoValue(i[1], ss)
		if err != nil {
			return eventActivityArgs{}, fmt.Errorf("%w: Activity %s keyword argument %s not serializable at event log index %d: %w", workflowengine.ErrInvalidArgs, name, key, idx, err)
		}
		e.kwargs[string(key)] = v
	}
	return e, nil
}

func (l *universeLibWF) execactivity(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	idx := l.events.Index()

	if len(args) == 0 {
		return nil, fmt.Errorf("%w: Missing activity function at event log index %d", workflowengine.ErrInvalidArgs, idx)
	}
	f, ok := args[0].(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("%w: Activity function is not callable at event log index %d", workflowengine.ErrInvalidArgs, idx)
	}
	args = args[1:]

	ss := stackset.NewAny()

	ea, err := l.serializeArgs(ss, idx, f.Name(), args, kwargs)
	if err != nil {
		return nil, err
	}
	if _, ok := l.events.Next(); ok {
		// TODO: compare args
	} else {
		l.events.Push(eventActivityArgsKey{}, ea)
	}

	if e, ok := l.events.Next(); ok {
		if e.Key != (eventActivityRetKey{}) {
			return nil, fmt.Errorf("Return event key mismatch for activity function %s at event log index %d", ea.name, idx)
		}
		er, ok := e.Value.(eventActivityRet)
		if !ok {
			return nil, fmt.Errorf("Return event value type mismatch for activity function %s at event log index %d", ea.name, idx)
		}
		if er.name != ea.name {
			return nil, fmt.Errorf("Activity function %s name mismatch of %s at event log index %d", ea.name, er.name, idx)
		}
		ret, err := goToStarlarkValue(er.ret, ss)
		if err != nil {
			return nil, fmt.Errorf("Failed deserializing activity function %s return value at event log index %d", ea.name, idx)
		}
		return ret, nil
	}

	ret, err := starlark.Call(t, f, args, kwargs)
	if err != nil {
		return nil, fmt.Errorf("Error executing activity %s at event log index %d: %w", ea.name, idx, err)
	}

	sRet, err := starlarkToGoValue(ret, ss)
	if err != nil {
		return nil, fmt.Errorf("%w: Activity %s return value not serializable at event log index %d: %w", workflowengine.ErrInvalidArgs, ea.name, idx, err)
	}
	l.events.Push(eventActivityRetKey{}, eventActivityRet{
		name: f.Name(),
		ret:  sRet,
	})

	return ret, nil
}
