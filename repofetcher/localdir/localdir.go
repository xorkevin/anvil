package localdir

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"

	"github.com/mitchellh/mapstructure"
	"xorkevin.dev/anvil/repofetcher"
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

	localDirOpts struct {
		Dir      string `mapstructure:"dir"`
		Checksum string `mapstructure:"checksum"`
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

func (f *Fetcher) Fetch(ctx context.Context, opts map[string]any) (fs.FS, error) {
	var fetchOpts localDirOpts
	if err := mapstructure.Decode(opts, &fetchOpts); err != nil {
		return nil, kerrors.WithMsg(err, "Invalid opts")
	}
	rfsys, err := fs.Sub(f.fsys, fetchOpts.Dir)
	if err != nil {
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed to get directory: %s", fetchOpts.Dir))
	}
	repopath := path.Join(f.dir, fetchOpts.Dir)
	rfsys = kfs.NewReadOnlyFS(kfs.New(rfsys, repopath))
	if fetchOpts.Checksum != "" {
		if ok, err := repofetcher.MerkelTreeVerify(rfsys, f.verifier, fetchOpts.Checksum); err != nil {
			return nil, kerrors.WithMsg(err, "Failed computing repo checksum")
		} else if !ok {
			return nil, kerrors.WithMsg(nil, "Repo failed integrity check")
		}
	}
	return rfsys, nil
}
