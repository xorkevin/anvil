package repofetcher_test

import (
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"xorkevin.dev/anvil/repofetcher"
	"xorkevin.dev/anvil/repofetcher/localdir"
	"xorkevin.dev/hunter2/h2streamhash"
	"xorkevin.dev/hunter2/h2streamhash/blake2bstream"
	"xorkevin.dev/kerrors"
)

type (
	mockLocalFile struct {
		name string
		data string
	}
)

func mockSetupDir(basedir string, dir string, files []mockLocalFile) error {
	for _, i := range files {
		fullPath := filepath.Join(
			filepath.FromSlash(basedir),
			filepath.FromSlash(dir),
			filepath.FromSlash(i.name),
		)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o777); err != nil {
			return kerrors.WithMsg(err, "Failed creating dir")
		}
		if err := os.WriteFile(fullPath, []byte(i.data), 0o644); err != nil {
			return kerrors.WithMsg(err, "Failed writing file")
		}
	}
	return nil
}

func TestFetcher(t *testing.T) {
	t.Parallel()

	tempCacheDir := t.TempDir()

	hasher := blake2bstream.NewHasher(blake2bstream.Config{})
	verifier := h2streamhash.NewVerifier()
	verifier.Register(hasher)

	t.Run("use local dir", func(t *testing.T) {
		t.Parallel()

		assert := require.New(t)

		files := []mockLocalFile{
			{
				name: "foo.txt",
				data: `
hello, world
`,
			},
			{
				name: "foobar/bar.txt",
				data: `
foobar
`,
			},
		}
		assert.NoError(mockSetupDir(tempCacheDir, "foo", files))

		m := repofetcher.Map{
			"localdir": localdir.New(path.Join(filepath.ToSlash(tempCacheDir), "foo")),
		}

		spec, err := m.Parse("localdir", []byte(`{"dir":"foo"}`))
		assert.NoError(err)
		assert.NotNil(spec)
		assert.Equal(repofetcher.Spec{
			Kind:     "localdir",
			RepoSpec: localdir.RepoSpec{},
		}, spec)
		repospeckey, err := spec.RepoSpec.Key()
		assert.NoError(err)
		assert.Equal("localdir", repospeckey)

		fsys, err := m.Fetch(context.Background(), spec)
		assert.NoError(err)
		assert.NotNil(fsys)

		for _, i := range files {
			data, err := fs.ReadFile(fsys, i.name)
			assert.NoError(err)
			assert.Equal([]byte(i.data), data)
		}

		sum, err := repofetcher.MerkelTreeHash(fsys, hasher)
		assert.NoError(err)

		assert.NoError(os.WriteFile(filepath.Join(tempCacheDir, "otherfile"), []byte("content"), 0o644))

		fsys, err = m.Fetch(context.Background(), spec)
		assert.NoError(err)
		assert.NotNil(fsys)

		for _, i := range files {
			data, err := fs.ReadFile(fsys, i.name)
			assert.NoError(err)
			assert.Equal([]byte(i.data), data)
		}

		ok, err := repofetcher.MerkelTreeVerify(fsys, verifier, sum)
		assert.NoError(err)
		assert.True(ok)
	})
}
