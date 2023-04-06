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
{
  "version": "xorkevin.dev/anvil/v1alpha1",
  "templates": [],
  "components": [],
}
`),
						Mode:    filemode,
						ModTime: now,
					},
				},
			},
			ConfigFile: "components/config.jsonnet",
			Files:      map[string]string{},
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
			assert.Len(components, 1)
		})
	}
}
