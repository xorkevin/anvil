package localdir

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"

	"xorkevin.dev/anvil/repofetcher"
	"xorkevin.dev/anvil/util/kjson"
	"xorkevin.dev/hunter2/h2streamhash"
	"xorkevin.dev/hunter2/h2streamhash/blake2bstream"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/kfs"
)

type (
	// Fetcher is a local dir fetcher
	Fetcher struct {
		fsys     fs.FS
		dir      string
		verifier *h2streamhash.Verifier
		Verbose  bool
	}

	// RepoSpec are local dir opts
	RepoSpec struct {
		Dir string `json:"dir"`
	}
)

// New creates a new local dir [*Fetcher] which is rooted at a particular file system
func New(dir string) *Fetcher {
	v := h2streamhash.NewVerifier()
	v.Register(blake2bstream.NewHasher(blake2bstream.Config{}))
	return &Fetcher{
		fsys:     os.DirFS(dir),
		dir:      dir,
		verifier: v,
		Verbose:  false,
	}
}

func (o RepoSpec) Key() (string, error) {
	cleaned := path.Clean(o.Dir)
	if cleaned != o.Dir {
		return "", kerrors.WithKind(nil, repofetcher.ErrInvalidRepoSpec, "Specified unsimplified dir")
	}
	return o.Dir, nil
}

func (f *Fetcher) Build(specbytes []byte) (repofetcher.RepoSpec, error) {
	var repospec RepoSpec
	if err := kjson.Unmarshal(specbytes, &repospec); err != nil {
		return nil, kerrors.WithKind(err, repofetcher.ErrInvalidRepoSpec, "Failed to parse spec bytes")
	}
	repospec.Dir = path.Clean(repospec.Dir)
	return repospec, nil
}

func (f *Fetcher) Fetch(ctx context.Context, spec repofetcher.RepoSpec) (fs.FS, error) {
	repospec, ok := spec.(RepoSpec)
	if !ok {
		return nil, kerrors.WithKind(nil, repofetcher.ErrInvalidRepoSpec, "Invalid spec type")
	}
	dir, err := repospec.Key()
	if err != nil {
		return nil, err
	}
	rfsys, err := fs.Sub(f.fsys, dir)
	if err != nil {
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed to get directory: %s", dir))
	}
	repopath := path.Join(f.dir, dir)
	return kfs.NewReadOnlyFS(kfs.New(rfsys, repopath)), nil
}
