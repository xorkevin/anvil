package jsonnetengine

import (
	"encoding/json"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_Engine(t *testing.T) {
	t.Parallel()

	now := time.Now()
	var filemode fs.FileMode = 0o644

	for _, tc := range []struct {
		Name     string
		Fsys     fs.FS
		Stl      string
		Main     string
		Args     map[string]any
		Expected any
	}{
		{
			Name: "executes jsonnet",
			Fsys: fstest.MapFS{
				"config.jsonnet": &fstest.MapFile{
					Data: []byte(`
{
  "hello": "world",
}
`),
					Mode:    filemode,
					ModTime: now,
				},
			},
			Stl:  "lib/anvil.libsonnet",
			Main: "config.jsonnet",
			Args: map[string]any{},
			Expected: map[string]any{
				"hello": "world",
			},
		},
	} {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			assert := require.New(t)

			jeng := New(tc.Fsys, tc.Stl)
			outbytes, err := jeng.Exec(tc.Main, tc.Args)
			assert.NoError(err)
			var out any
			assert.NoError(json.Unmarshal(outbytes, &out))
			assert.Equal(tc.Expected, out)
		})
	}
}
