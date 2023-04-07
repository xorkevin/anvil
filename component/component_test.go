package component

import (
	"context"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"
	"xorkevin.dev/anvil/confengine"
	"xorkevin.dev/anvil/confengine/jsonnetengine"
	"xorkevin.dev/anvil/repofetcher"
	"xorkevin.dev/anvil/repofetcher/localdir"
	"xorkevin.dev/kfs/kfstest"
)

func TestGenerate(t *testing.T) {
	t.Parallel()

	now := time.Now()
	var filemode fs.FileMode = 0o644

	for _, tc := range []struct {
		Name       string
		LocalFS    fs.FS
		ConfigFile string
		Files      map[string]string
	}{
		{
			Name: "full",
			LocalFS: &kfstest.MapFS{
				Fsys: fstest.MapFS{
					"components/config.jsonnet": &fstest.MapFile{
						Data: []byte(`
local anvil = import 'anvil.libsonnet';

local output = 'anvil_out';

{
  version: 'xorkevin.dev/anvil/v1alpha1',
  templates: [
    {
      kind: 'jsonnetstr',
      path: 'foo.txt',
      args: {
        msg: 'hello, world',
      },
      output: anvil.pathJoin([output, 'foo.txt']),
    },
  ],
  components: [
    {
      path: 'subcomp/config.jsonnet',
      args: {
        output: anvil.pathJoin([output, 'bar']),
      },
    },
  ],
}
`),
						Mode:    filemode,
						ModTime: now,
					},
					"components/foo.txt": &fstest.MapFile{
						Data: []byte(`
local anvil = import 'anvil.libsonnet';
local args = anvil.getArgs();

@'Greetings. %(msg)s' % args
`),
						Mode:    filemode,
						ModTime: now,
					},
					"components/subcomp/config.jsonnet": &fstest.MapFile{
						Data: []byte(`
local anvil = import 'anvil.libsonnet';

local args = anvil.getArgs();
local output = args.output;

{
  version: 'xorkevin.dev/anvil/v1alpha1',
  templates: [
    {
      kind: 'jsonnetstr',
      path: 'foobar.txt',
      args: {
        value: 'foo bar baz',
      },
      output: anvil.pathJoin([output, 'baz.txt']),
    },
  ],
  components: [],
}
`),
						Mode:    filemode,
						ModTime: now,
					},
					"components/subcomp/foobar.txt": &fstest.MapFile{
						Data: []byte(`
local anvil = import 'anvil.libsonnet';

local args = anvil.getArgs();

@'Arg value: %(value)s' % args
`),
						Mode:    filemode,
						ModTime: now,
					},
				},
			},
			ConfigFile: "components/config.jsonnet",
			Files: map[string]string{
				"anvil_out/foo.txt":     "Greetings. hello, world\n",
				"anvil_out/bar/baz.txt": "Arg value: foo bar baz\n",
			},
		},
	} {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			assert := require.New(t)

			cache := NewCache(
				repofetcher.NewCache(
					repofetcher.Map{
						"localdir": localdir.New(tc.LocalFS),
					},
					map[string]struct{}{
						"localdir": {},
					},
					nil,
				),
				confengine.Map{
					configKindJsonnet: jsonnetengine.Builder{},
					"jsonnetstr":      jsonnetengine.Builder{jsonnetengine.OptStrOut(true)},
				},
			)

			components, err := ParseComponents(context.Background(), cache, repofetcher.Spec{Kind: "localdir", RepoSpec: localdir.RepoSpec{}}, tc.ConfigFile)
			assert.NoError(err)
			assert.Len(components, 2)

			outputfs := &kfstest.MapFS{
				Fsys: fstest.MapFS{},
			}
			assert.NoError(WriteComponents(context.Background(), cache, outputfs, components))

			for k, v := range tc.Files {
				assert.NotNil(outputfs.Fsys[k])
				assert.Equal(v, string(outputfs.Fsys[k].Data))
			}
		})
	}
}
