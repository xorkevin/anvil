package starlarkengine

import (
	"context"
	"errors"
	"fmt"

	"go.starlark.net/starlark"
	"xorkevin.dev/anvil/util/stackset"
	"xorkevin.dev/anvil/workflowengine"
)

type (
	universeLibWF struct {
		events *workflowengine.EventHistory
	}

	activityEventKey struct {
		name string
	}

	activityEvent struct {
		t      *starlark.Thread
		f      starlark.Callable
		args   starlark.Tuple
		kwargs []starlark.Tuple
	}
)

func (l universeLibWF) mod() starlark.StringDict {
	return starlark.StringDict{
		"execactivity": starlark.NewBuiltin("execactivity", l.execactivity),
	}
}

func (e activityEvent) Key() any {
	return activityEventKey{
		name: e.f.Name(),
	}
}

func (e activityEvent) Serialize() (any, error) {
	args := make([]any, 0, len(e.args))
	kwargs := make(map[string]any, len(e.kwargs))

	ss := stackset.NewAny()
	for n, i := range e.args {
		v, err := starlarkToGoValue(i, ss)
		if err != nil {
			return nil, fmt.Errorf("Positional argument %d not serializable: %w", n, err)
		}
		args = append(args, v)
	}
	for _, i := range e.kwargs {
		if len(i) != 2 {
			return nil, fmt.Errorf("Malformed keyword argument")
		}
		key, ok := i[0].(starlark.String)
		if !ok {
			return nil, fmt.Errorf("Malformed keyword argument")
		}
		v, err := starlarkToGoValue(i[1], ss)
		if err != nil {
			return nil, fmt.Errorf("Keyword argument %s not serializable: %w", key, err)
		}
		kwargs[string(key)] = v
	}

	return map[string]any{
		"name":   e.f.Name(),
		"args":   args,
		"kwargs": kwargs,
	}, nil
}

func (e activityEvent) Exec(ctx context.Context) (any, error) {
	ret, err := starlark.Call(e.t, e.f, e.args, e.kwargs)
	if err != nil {
		return nil, fmt.Errorf("Error calling activity function: %w", err)
	}
	v, err := starlarkToGoValue(ret, stackset.NewAny())
	if err != nil {
		return nil, fmt.Errorf("Activity function return value not serializable: %w", err)
	}
	return v, nil
}

func (l *universeLibWF) execactivity(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx, ok := t.Local("ctx").(context.Context)
	if !ok {
		return nil, errors.New("No thread ctx")
	}

	idx := l.events.Index()
	if len(args) == 0 {
		return nil, fmt.Errorf("%w: Missing activity function at event log index %d", workflowengine.ErrInvalidArgs, idx)
	}
	f, ok := args[0].(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("%w: Activity function is not callable at event log index %d", workflowengine.ErrInvalidArgs, idx)
	}
	args = args[1:]

	value, err := l.events.ExecActivity(ctx, activityEvent{
		t:      t,
		f:      f,
		args:   args,
		kwargs: kwargs,
	})
	if err != nil {
		return nil, err
	}
	ret, err := goToStarlarkValue(value, stackset.NewAny())
	if err != nil {
		return nil, fmt.Errorf("Failed deserializing activity function %s return value at event log index %d", f.Name(), idx)
	}
	return ret, nil
}
