package jsonnetengine

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"
	"xorkevin.dev/anvil/util/kjson"
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
local args = anvil.getargs();

local world = import 'subdir/world.libsonnet';

{
  "hello": args.name,
  "str": anvil.jsonMarshal({
    "foo": "bar",
  }),
  "foo": anvil.jsonUnmarshal('{"foo":1}'),
  "bar": 2,
  "name": anvil.pathJoin(['abc', 'def']),
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
				"foo": map[string]any{
					"foo": json.Number("1"),
				},
				"bar":  json.Number("2"),
				"name": "abc/def",
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
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			assert := require.New(t)

			eng, err := Builder{OptStrOut(tc.RawString)}.Build(tc.Fsys)
			assert.NoError(err)
			out, err := eng.Exec(context.Background(), tc.Main, tc.Args, nil)
			assert.NoError(err)
			var b bytes.Buffer
			_, err = io.Copy(&b, out)
			assert.NoError(err)
			if tc.RawString {
				assert.Equal(tc.Expected, b.String())
			} else {
				var out any
				assert.NoError(kjson.Unmarshal(b.Bytes(), &out))
				assert.Equal(tc.Expected, out)
			}
		})
	}
}
