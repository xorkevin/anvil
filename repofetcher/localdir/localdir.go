package localdir

import (
	"context"
	"io/fs"

	"xorkevin.dev/anvil/repofetcher"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/kfs"
)

type (
	// Fetcher is a local dir fetcher
	Fetcher struct {
		fsys fs.FS
	}

	// RepoSpec are local dir opts
	RepoSpec struct{}
)

// New creates a new local dir [*Fetcher] which is rooted at a particular file system
func New(fsys fs.FS) *Fetcher {
	return &Fetcher{
		fsys: kfs.NewReadOnlyFS(fsys),
	}
}

func (o RepoSpec) Key() (string, error) {
	return "localdir", nil
}

func (f *Fetcher) Parse(specbytes []byte) (repofetcher.RepoSpec, error) {
	return RepoSpec{}, nil
}

func (f *Fetcher) Fetch(ctx context.Context, spec repofetcher.RepoSpec) (fs.FS, error) {
	if _, ok := spec.(RepoSpec); !ok {
		return nil, kerrors.WithKind(nil, repofetcher.ErrInvalidRepoSpec, "Invalid spec type")
	}
	return f.fsys, nil
}
