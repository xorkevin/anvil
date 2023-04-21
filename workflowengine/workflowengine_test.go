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
		count int
	}

	mockActivityKey struct {
		key string
	}

	mockActivity struct {
		e    *mockEngine
		key  string
		args map[string]any
	}
)

func (a mockActivity) Key() any {
	return mockActivityKey{key: a.key}
}

func (a mockActivity) Serialize() (any, error) {
	return a.args, nil
}

func (a mockActivity) Exec(ctx context.Context) (any, error) {
	a.e.count++
	if a.e.count < 2 {
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

func (e *mockEngine) Exec(ctx context.Context, events *EventHistory, name string, fn string, args map[string]any, w io.Writer) (any, error) {
	return events.ExecActivity(ctx, mockActivity{
		e:    e,
		key:  name + "." + fn,
		args: args,
	})
}

func TestWorkflowEngine(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		Name     string
		Filename string
		Main     string
		Args     map[string]any
		Expected any
	}{
		{
			Name:     "executes workflow engine",
			Filename: "foo.mockengine",
			Main:     "main",
			Args: map[string]any{
				"hello": "world",
			},
			Expected: "foo.mockengine.main: {\"hello\":\"world\"}",
		},
	} {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			assert := require.New(t)

			engines := Map{
				"mockengine": BuilderFunc(func(fsys fs.FS) (WorkflowEngine, error) {
					return &mockEngine{
						count: 0,
					}, nil
				}),
			}
			eng, err := engines.Build("mockengine", nil)
			assert.NoError(err)
			v, err := ExecWorkflow(context.Background(), eng, tc.Filename, tc.Main, tc.Args, WorkflowOpts{
				Log:        klog.New(klog.OptHandler(klog.NewJSONSlogHandler(io.Discard))),
				MaxRetries: 3,
				MinBackoff: 0,
				MaxBackoff: 0,
			})
			assert.NoError(err)
			assert.Equal(tc.Expected, v)
		})
	}
}
