package staticfile

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
		File     string
		Expected string
	}{
		{
			Name: "returns file",
			Fsys: fstest.MapFS{
				"foo.txt": &fstest.MapFile{
					Data:    []byte(`foobar`),
					Mode:    filemode,
					ModTime: now,
				},
				"bar.txt": &fstest.MapFile{
					Data:    []byte(`barfoo`),
					Mode:    filemode,
					ModTime: now,
				},
			},
			File:     "foo.txt",
			Expected: "foobar",
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			assert := require.New(t)

			eng, err := Builder{}.Build(tc.Fsys)
			assert.NoError(err)
			out, err := eng.Exec(context.Background(), tc.File, nil, nil)
			assert.NoError(err)
			var b bytes.Buffer
			_, err = io.Copy(&b, out)
			assert.NoError(err)
			assert.Equal(tc.Expected, b.String())
		})
	}
}
