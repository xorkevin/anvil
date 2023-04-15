package gotmplengine

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEngine(t *testing.T) {
	t.Parallel()

	now := time.Now()
	var filemode fs.FileMode = 0o644

	for _, tc := range []struct {
		Name     string
		Fsys     fs.FS
		Args     map[string]any
		File     string
		Expected string
	}{
		{
			Name: "executes go template",
			Fsys: fstest.MapFS{
				"foo.txt.tmpl": &fstest.MapFile{
					Data:    []byte(`Hello, {{.target}}`),
					Mode:    filemode,
					ModTime: now,
				},
			},
			Args: map[string]any{
				"target": "world",
			},
			File:     "foo.txt.tmpl",
			Expected: "Hello, world",
		},
	} {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			assert := require.New(t)

			eng, err := Builder{}.Build(tc.Fsys)
			assert.NoError(err)
			out, err := eng.Exec(context.Background(), tc.File, tc.Args, nil)
			assert.NoError(err)
			var b bytes.Buffer
			_, err = io.Copy(&b, out)
			assert.NoError(err)
			assert.Equal(tc.Expected, b.String())
		})
	}
}
