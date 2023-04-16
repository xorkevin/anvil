package starlarkengine

import (
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEngine(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

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
	}{
		{
			Name: "executes starlark",
			Fsys: fstest.MapFS{
				"main.star": &fstest.MapFile{
					Data: []byte(`
load("anvil:std", "writefile")
load("subdir/hello.star", "hello_msg")

def main(args):
  file = args["file"]
  name = args["name"]
  if file == None or not file.startswith("/tmp/"):
    fail("Invalid file")
  writefile(file, hello_msg(name))
  return True
`),
					Mode:    filemode,
					ModTime: now,
				},
				"subdir/hello.star": &fstest.MapFile{
					Data: []byte(`
load("anvil:std", "gotmpl")
def hello_msg(name):
  return gotmpl("""Hello, {{.name}}""", {
    "name": name,
  })
`),
					Mode:    filemode,
					ModTime: now,
				},
			},
			File: "main.star",
			Main: "main",
			Args: map[string]any{
				"file": path.Join(filepath.ToSlash(tempDir), "out.conf"),
				"name": "world",
			},
			Expected: true,
			ExpectedFiles: map[string]string{
				"out.conf": "Hello, world",
			},
		},
	} {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			assert := require.New(t)

			eng, err := Builder{}.Build(tc.Fsys)
			assert.NoError(err)
			out, err := eng.Exec(context.Background(), tc.File, tc.Main, tc.Args, nil)
			assert.NoError(err)
			assert.Equal(tc.Expected, out)
			for k, v := range tc.ExpectedFiles {
				b, err := os.ReadFile(filepath.Join(tempDir, filepath.FromSlash(k)))
				assert.NoError(err)
				assert.Equal(v, string(b))
			}
		})
	}
}
