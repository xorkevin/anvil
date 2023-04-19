package workflowengine

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type (
	mockEngine struct{}
)

func (e mockEngine) Exec(ctx context.Context, events *EventHistory, name string, fn string, args map[string]any, w io.Writer) (any, error) {
	j, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString(name)
	b.WriteString(".")
	b.WriteString(fn)
	b.WriteString(": ")
	if _, err := b.Write(j); err != nil {
		return nil, err
	}
	return b.String(), nil
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
					return mockEngine{}, nil
				}),
			}
			eng, err := engines.Build("mockengine", nil)
			assert.NoError(err)
			v, err := eng.Exec(context.Background(), NewEventHistory(), tc.Filename, tc.Main, tc.Args, nil)
			assert.NoError(err)
			assert.Equal(tc.Expected, v)
		})
	}
}
