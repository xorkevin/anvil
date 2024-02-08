package workflowengine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"xorkevin.dev/klog"
)

type (
	mockEngine struct {
		count1 int
		count2 int
	}

	mockActivityKey struct {
		key string
	}

	mockActivity1 struct {
		e   *mockEngine
		key string
	}

	mockActivity2 struct {
		e    *mockEngine
		key  string
		args map[string]any
	}
)

func (a mockActivity1) Key() any {
	return mockActivityKey{key: a.key}
}

func (a mockActivity1) Serialize() (any, error) {
	return nil, nil
}

func (a mockActivity1) Exec(ctx context.Context) (any, error) {
	a.e.count1++
	if a.e.count1 < 2 {
		return nil, errors.New("Temp test error")
	}
	return 1, nil
}

func (a mockActivity2) Key() any {
	return mockActivityKey{key: a.key}
}

func (a mockActivity2) Serialize() (any, error) {
	return a.args, nil
}

func (a mockActivity2) Exec(ctx context.Context) (any, error) {
	a.e.count2++
	if a.e.count2 < 2 {
		return nil, errors.New("Temp test error")
	}
	j, err := json.Marshal(a.args)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString(a.key)
	b.WriteString(": ")
	if _, err := b.Write(j); err != nil {
		return nil, err
	}
	return b.String(), nil
}

func (e *mockEngine) Exec(ctx context.Context, events *EventHistory, name string, args map[string]any, w io.Writer) (any, error) {
	if _, err := events.ExecActivity(ctx, mockActivity1{
		e:   e,
		key: name,
	}); err != nil {
		return nil, err
	}
	return events.ExecActivity(ctx, mockActivity2{
		e:    e,
		key:  name,
		args: args,
	})
}

func TestWorkflowEngine(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		Name     string
		Filename string
		Args     map[string]any
		Expected any
	}{
		{
			Name:     "executes workflow engine",
			Filename: "foo.mockengine",
			Args: map[string]any{
				"hello": "world",
			},
			Expected: "foo.mockengine: {\"hello\":\"world\"}",
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			assert := require.New(t)

			engines := Map{
				"mockengine": BuilderFunc(func(fsys fs.FS) (WorkflowEngine, error) {
					return &mockEngine{
						count1: 0,
						count2: 0,
					}, nil
				}),
			}
			eng, err := engines.Build("mockengine", nil)
			assert.NoError(err)
			e, ok := eng.(*mockEngine)
			assert.True(ok)
			v, err := ExecWorkflow(context.Background(), eng, tc.Filename, tc.Args, WorkflowOpts{
				Log:        klog.Discard{},
				MaxRetries: 5,
				MinBackoff: 0,
				MaxBackoff: 0,
			})
			assert.NoError(err)
			assert.Equal(tc.Expected, v)
			assert.Equal(2, e.count1)
			assert.Equal(2, e.count2)
		})
	}
}
