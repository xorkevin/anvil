package gitfetcher

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"xorkevin.dev/anvil/repofetcher"
	"xorkevin.dev/hunter2/h2streamhash"
	"xorkevin.dev/hunter2/h2streamhash/blake2bstream"
	"xorkevin.dev/kerrors"
)

type (
	mockGitCmd struct {
		repo     string
		files    []mockGitFile
		gitFiles []mockGitFile
	}

	mockGitFile struct {
		name string
		data string
	}
)

func (m *mockGitCmd) GitClone(ctx context.Context, workingDir string, repodir string, repospec RepoSpec) error {
	if repospec.Repo != m.repo {
		return kerrors.WithMsg(nil, "Unknown repo")
	}
	for _, i := range m.files {
		fullPath := filepath.Join(
			filepath.FromSlash(workingDir),
			filepath.FromSlash(repodir),
			filepath.FromSlash(i.name),
		)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o777); err != nil {
			return kerrors.WithMsg(err, "Failed creating dir")
		}
		if err := os.WriteFile(fullPath, []byte(i.data), 0o644); err != nil {
			return kerrors.WithMsg(err, "Failed writing file")
		}
	}
	for _, i := range m.gitFiles {
		fullPath := filepath.Join(
			filepath.FromSlash(workingDir),
			filepath.FromSlash(repodir),
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

func Test_Fetcher(t *testing.T) {
	t.Parallel()

	tempCacheDir := t.TempDir()

	hasher := blake2bstream.NewHasher(blake2bstream.Config{})
	verifier := h2streamhash.NewVerifier()
	verifier.Register(hasher)

	t.Run("use cached git repo", func(t *testing.T) {
		t.Parallel()

		assert := require.New(t)

		repo := "git@example.com:example/repo.git"

		files := []mockGitFile{
			{
				name: "foo.txt",
				data: `
hello, world
`,
			},
			{
				name: "foo/bar.txt",
				data: `
foobar
`,
			},
		}
		gitFiles := []mockGitFile{
			{
				name: ".git/ignorethis",
				data: `
should be ignored
`,
			},
		}

		fetcher := New(tempCacheDir, OptGitCmd(&mockGitCmd{
			repo:     repo,
			files:    files,
			gitFiles: gitFiles,
		}))

		repodir := "git%40example.com%3Aexample%2Frepo.git@test"
		repospec, err := fetcher.Parse([]byte(`{"repo":"` + repo + `","tag":"test"}`))
		assert.NoError(err)
		assert.Equal(RepoSpec{
			Repo: repo,
			Tag:  "test",
		}, repospec)
		repospeckey, err := repospec.Key()
		assert.NoError(err)
		assert.Equal(repodir, repospeckey)

		fsys, err := fetcher.Fetch(context.Background(), RepoSpec{
			Repo: repo,
			Tag:  "test",
		})
		assert.NoError(err)
		assert.NotNil(fsys)

		for _, i := range files {
			data, err := fs.ReadFile(fsys, i.name)
			assert.NoError(err)
			assert.Equal([]byte(i.data), data)
		}

		sum, err := repofetcher.MerkelTreeHash(fsys, hasher)
		assert.NoError(err)

		repoinfo, err := os.Stat(filepath.Join(tempCacheDir, repodir))
		assert.NoError(err)
		assert.True(repoinfo.IsDir())
		assert.NoError(os.WriteFile(filepath.Join(tempCacheDir, repodir, ".git", "otherfile"), []byte("content"), 0o644))

		fsys, err = fetcher.Fetch(context.Background(), RepoSpec{
			Repo: repo,
			Tag:  "test",
		})
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
