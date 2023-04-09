package jsonnetengine

import (
	"encoding/json"
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
		Name      string
		Fsys      fs.FS
		Main      string
		Args      map[string]any
		RawString bool
		Expected  any
	}{
		{
			Name: "executes jsonnet",
			Fsys: fstest.MapFS{
				"config.jsonnet": &fstest.MapFile{
					Data: []byte(`
local anvil = import 'anvil:std';
local args = anvil.getArgs();

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
		{
			Name: "outputs raw string",
			Fsys: fstest.MapFS{
				"config.jsonnet": &fstest.MapFile{
					Data: []byte(`
"hello, world"
`),
					Mode:    filemode,
					ModTime: now,
				},
			},
			Main:      "config.jsonnet",
			RawString: true,
			Expected:  "hello, world\n",
		},
	} {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			assert := require.New(t)

			eng, err := Builder{OptStrOut(tc.RawString)}.Build(tc.Fsys)
			assert.NoError(err)
			outbytes, err := eng.Exec(tc.Main, tc.Args)
			assert.NoError(err)
			if tc.RawString {
				assert.Equal(tc.Expected, string(outbytes))
			} else {
				var out any
				assert.NoError(json.Unmarshal(outbytes, &out))
				assert.Equal(tc.Expected, out)
			}
		})
	}
}
