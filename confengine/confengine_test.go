package confengine

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/require"
)

type (
	mockEngine struct{}
)

func (e mockEngine) Exec(ctx context.Context, name string, args map[string]any, w io.Writer) (io.ReadCloser, error) {
	j, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	io.WriteString(&b, name)
	io.WriteString(&b, ": ")
	if _, err := b.Write(j); err != nil {
		return nil, err
	}
	return io.NopCloser(&b), nil
}

func TestConfEngine(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		Name     string
		Filename string
		Args     map[string]any
		Expected any
	}{
		{
			Name:     "executes confengine",
			Filename: "foo.mockengine",
			Args: map[string]any{
				"hello": "world",
			},
			Expected: map[string]any{
				"hello": "world",
			},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			assert := require.New(t)

			engines := Map{
				"mockengine": BuilderFunc(func(fsys fs.FS) (ConfEngine, error) {
					return mockEngine{}, nil
				}),
			}
			eng, err := engines.Build("mockengine", nil)
			assert.NoError(err)
			outreader, err := eng.Exec(context.Background(), tc.Filename, tc.Args, nil)
			assert.NoError(err)
			var b bytes.Buffer
			_, err = io.Copy(&b, outreader)
			assert.NoError(err)
			assert.True(bytes.HasPrefix(b.Bytes(), []byte(tc.Filename+": ")))
			outbytes := b.Bytes()[len(tc.Filename)+2:]
			var out any
			assert.NoError(json.Unmarshal(outbytes, &out))
			assert.Equal(tc.Expected, out)
		})
	}
}
