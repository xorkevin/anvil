package starlarkengine

import (
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEngine(t *testing.T) {
	t.Parallel()

	assert := require.New(t)

	tempDir := t.TempDir()

	assert.NoError(os.WriteFile(filepath.Join(tempDir, "foo.txt"), []byte(`foo`), 0o644))

	now := time.Now()
	var filemode fs.FileMode = 0o644

	for _, tc := range []struct {
		Name          string
		Fsys          fs.FS
		File          string
		Main          string
		Args          map[string]any
		Expected      any
		ExpectedFiles map[string]string
		Log           string
	}{
		{
			Name: "executes starlark",
			Fsys: fstest.MapFS{
				"main.star": &fstest.MapFile{
					Data: []byte(`
load("anvil:std", "os", "json")
load("subdir/hello.star", "hello_msg")

def main(args):
  file = args["file"]
  if not file.startswith("/tmp/"):
    fail("Invalid file")
  foo = os.readfile(args["inp"])
  os.writefile(file, json.marshal({ "msg": hello_msg(args["name"]) }))
  return json.mergepatch(
    json.unmarshal("""{ "a": 1, "b": "b" }"""),
    { "b": foo, "c": "bar" },
  )
`),
					Mode:    filemode,
					ModTime: now,
				},
				"subdir/hello.star": &fstest.MapFile{
					Data: []byte(`
load("anvil:std", "template", "os", "path")

def hello_msg(name):
  print("writing message from dir {}".format(__anvil_moddir__))
  tpl = os.readmodfile(path.join([__anvil_moddir__, "msg.tmpl"]))
  return template.gotpl(tpl, { "name": name })
`),
					Mode:    filemode,
					ModTime: now,
				},
				"subdir/msg.tmpl": &fstest.MapFile{
					Data:    []byte(`Hello, {{.name}}`),
					Mode:    filemode,
					ModTime: now,
				},
			},
			File: "main.star",
			Main: "main",
			Args: map[string]any{
				"file": path.Join(filepath.ToSlash(tempDir), "out.json"),
				"inp":  path.Join(filepath.ToSlash(tempDir), "foo.txt"),
				"name": "world",
			},
			Expected: map[string]any{
				"a": 1,
				"b": "foo",
				"c": "bar",
			},
			ExpectedFiles: map[string]string{
				"out.json": "{\"msg\":\"Hello, world\"}\n",
			},
			Log: "writing message from dir subdir\n",
		},
	} {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			assert := require.New(t)

			eng, err := Builder{}.Build(tc.Fsys)
			assert.NoError(err)
			var log strings.Builder
			out, err := eng.Exec(context.Background(), tc.File, tc.Main, tc.Args, &log)
			assert.NoError(err)
			assert.Equal(tc.Expected, out)
			assert.Equal(tc.Log, log.String())
			for k, v := range tc.ExpectedFiles {
				b, err := os.ReadFile(filepath.Join(tempDir, filepath.FromSlash(k)))
				assert.NoError(err)
				assert.Equal(v, string(b))
			}
		})
	}
}
