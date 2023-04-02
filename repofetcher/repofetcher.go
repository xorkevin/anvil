package repofetcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"

	"xorkevin.dev/hunter2/h2streamhash"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/kfs"
)

var (
	// ErrUnknownKind is returned when the repo kind is not supported
	ErrUnknownKind errUnknownKind
	// ErrInvalidRepoSpec is returned when the repo spec is invalid
	ErrInvalidRepoSpec errInvalidRepoSpec
	// ErrInvalidCache is returned when the cached repo is invalid
	ErrInvalidCache errInvalidCache
	// ErrNetworkRequired is returned when the network is required to complete the operation
	ErrNetworkRequired errNetworkRequired
)

type (
	errUnknownKind     struct{}
	errInvalidRepoSpec struct{}
	errInvalidCache    struct{}
	errNetworkRequired struct{}
)

func (e errUnknownKind) Error() string {
	return "Unknown repo kind"
}

func (e errInvalidRepoSpec) Error() string {
	return "Invalid repo spec"
}

func (e errInvalidCache) Error() string {
	return "Invalid cache"
}

func (e errNetworkRequired) Error() string {
	return "Network required"
}

type (
	// RepoSpec are repo specific options
	RepoSpec interface {
		Key() (string, error)
	}

	// Spec is a repo specification
	Spec struct {
		Kind     string
		RepoSpec RepoSpec
	}

	// RepoFetcher fetches a repo of a particular kind
	RepoFetcher interface {
		Parse(repobytes []byte) (RepoSpec, error)
		Fetch(ctx context.Context, repospec RepoSpec) (fs.FS, error)
	}

	// Map is a map from kinds to repo fetchers
	Map map[string]RepoFetcher
)

func (m Map) Parse(kind string, repobytes []byte) (Spec, error) {
	f, ok := m[kind]
	if !ok {
		return Spec{}, ErrUnknownKind
	}
	repospec, err := f.Parse(repobytes)
	if err != nil {
		return Spec{}, kerrors.WithMsg(err, "Failed to build repo spec")
	}
	if _, err := repospec.Key(); err != nil {
		return Spec{}, kerrors.WithMsg(err, "Invalid repo spec")
	}
	return Spec{
		Kind:     kind,
		RepoSpec: repospec,
	}, nil
}

func (m Map) Fetch(ctx context.Context, spec Spec) (fs.FS, error) {
	f, ok := m[spec.Kind]
	if !ok {
		return nil, ErrUnknownKind
	}
	fsys, err := f.Fetch(ctx, spec.RepoSpec)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to fetch repo")
	}
	return fsys, nil
}

type (
	merkelHasher interface {
		Hash() (h2streamhash.Hash, error)
	}
)

func merkelHash(
	fsys fs.FS,
	p string,
	entry fs.DirEntry,
	hasher merkelHasher,
) (h2streamhash.Hash, error) {
	hash, err := hasher.Hash()
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to construct hash")
	}

	if entry.Type()&(^(fs.ModeSymlink | fs.ModeDir)) != 0 {
		// entry is not a regular file, symlink, or dir
		return nil, nil
	}

	notEmpty, err := func() (_ bool, retErr error) {
		if entry.Type()&fs.ModeSymlink != 0 {
			// symlink
			dest, err := kfs.ReadLink(fsys, p)
			if err != nil {
				return false, kerrors.WithMsg(err, fmt.Sprintf("Failed to read symlink: %s", p))
			}
			if _, err := io.WriteString(hash, dest); err != nil {
				return false, kerrors.WithMsg(err, "Failed to write to hash")
			}
			return true, nil
		} else if !entry.IsDir() {
			// regular file
			f, err := fsys.Open(p)
			if err != nil {
				return false, kerrors.WithMsg(err, fmt.Sprintf("Failed to open file: %s", p))
			}
			defer func() {
				if err := f.Close(); err != nil {
					retErr = errors.Join(retErr, kerrors.WithMsg(err, "Failed to close open file"))
				}
			}()
			if _, err := io.Copy(hash, f); err != nil {
				return false, kerrors.WithMsg(err, fmt.Sprintf("Failed reading file: %s", p))
			}
			return true, nil
		}
		// directory
		entries, err := fs.ReadDir(fsys, p)
		if err != nil {
			return false, kerrors.WithMsg(err, fmt.Sprintf("Failed reading dir: %s", p))
		}
		hasEntry := false
		for _, i := range entries {
			h, err := merkelHash(fsys, path.Join(p, i.Name()), i, hasher)
			if err != nil {
				return false, err
			}
			if h == nil {
				continue
			}
			hasEntry = true
			if _, err := io.WriteString(hash, i.Name()); err != nil {
				return false, kerrors.WithMsg(err, "Failed to write to hash")
			}
			if _, err := hash.Write([]byte{0}); err != nil {
				return false, kerrors.WithMsg(err, "Failed to write to hash")
			}
			if _, err := io.WriteString(hash, h.Sum()); err != nil {
				return false, kerrors.WithMsg(err, "Failed to write to hash")
			}
			if _, err := hash.Write([]byte{0}); err != nil {
				return false, kerrors.WithMsg(err, "Failed to write to hash")
			}
		}
		return hasEntry, nil
	}()
	if err != nil {
		return nil, err
	}
	if !notEmpty {
		return nil, nil
	}

	if err := hash.Close(); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to close hash")
	}
	return hash, nil
}

func merkelTreeHash(
	fsys fs.FS,
	hasher merkelHasher,
) (h2streamhash.Hash, error) {
	info, err := fs.Stat(fsys, ".")
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to read dir")
	}
	h, err := merkelHash(fsys, ".", fs.FileInfoToDirEntry(info), hasher)
	if err != nil {
		return nil, err
	}
	if h == nil {
		h, err = hasher.Hash()
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to construct hash")
		}
	}
	return h, nil
}

func MerkelTreeHash(
	fsys fs.FS,
	hasher h2streamhash.Hasher,
) (string, error) {
	h, err := merkelTreeHash(fsys, hasher)
	if err != nil {
		return "", err
	}
	return h.Sum(), nil
}

type (
	verifierHasher struct {
		verifier *h2streamhash.Verifier
		checksum string
	}
)

func (v *verifierHasher) Hash() (h2streamhash.Hash, error) {
	return v.verifier.Verify(v.checksum)
}

func MerkelTreeVerify(
	fsys fs.FS,
	verifier *h2streamhash.Verifier,
	checksum string,
) (bool, error) {
	h, err := merkelTreeHash(fsys, &verifierHasher{verifier: verifier, checksum: checksum})
	if err != nil {
		return false, err
	}
	return h.Verify(checksum)
}
