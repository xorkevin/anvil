package jsonnetengine

import (
	"encoding/json"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"
	"xorkevin.dev/anvil/confengine"
)

func Test_Engine(t *testing.T) {
	t.Parallel()

	now := time.Now()
	var filemode fs.FileMode = 0o644

	for _, tc := range []struct {
		Name     string
		Fsys     fs.FS
		Std      string
		Main     string
		Args     map[string]any
		Expected any
	}{
		{
			Name: "executes jsonnet",
			Fsys: fstest.MapFS{
				"config.jsonnet": &fstest.MapFile{
					Data: []byte(`
local anvil = import 'anvilstd.libsonnet';
local args = anvil.envArgs();

local world = import 'subdir/world.libsonnet';

{
  "hello": args.name,
  "str": anvil.jsonMarshal({
    "foo": "bar",
  }),
  "obj": anvil.jsonMergePatch(
    {
      "foo": {
        "bar": "baz",
      },
      "hello": "world",
    },
    {
      "foo": {
        "bar": world.name,
      },
    },
  ),
}
`),
					Mode:    filemode,
					ModTime: now,
				},
				"subdir/world.libsonnet": &fstest.MapFile{
					Data: []byte(`
local vars = import '/vars.libsonnet';

{
  "name": vars.worldname,
}
`),
					Mode:    filemode,
					ModTime: now,
				},
				"vars.libsonnet": &fstest.MapFile{
					Data: []byte(`
{
  "worldname": "foo",
}
`),
					Mode:    filemode,
					ModTime: now,
				},
			},
			Std:  "anvilstd.libsonnet",
			Main: "config.jsonnet",
			Args: map[string]any{
				"name": "world",
			},
			Expected: map[string]any{
				"hello": "world",
				"str":   "{\"foo\":\"bar\"}\n",
				"obj": map[string]any{
					"foo": map[string]any{
						"bar": "foo",
					},
					"hello": "world",
				},
			},
		},
	} {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			assert := require.New(t)

			jeng := New(tc.Fsys, tc.Std, nil)
			{
				var _ confengine.ConfEngine = jeng
			}
			outbytes, err := jeng.Exec(tc.Main, tc.Args)
			assert.NoError(err)
			var out any
			assert.NoError(json.Unmarshal(outbytes, &out))
			assert.Equal(tc.Expected, out)
		})
	}
}
