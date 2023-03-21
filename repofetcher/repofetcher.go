package repofetcher

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"

	"xorkevin.dev/hunter2/h2streamhash"
	"xorkevin.dev/kerrors"
)

var (
	// ErrorUnknownKind is returned when the repo kind is not supported
	ErrorUnknownKind errUnknownKind
	// ErrorInvalidRepoSpec is returned when the repo spec is invalid
	ErrorInvalidRepoSpec errInvalidRepoSpec
	// ErrorInvalidCache is returned when the cached repo is invalid
	ErrorInvalidCache errInvalidCache
	// ErrorNetworkRequired is returned when the network is required to complete the operation
	ErrorNetworkRequired errNetworkRequired
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
	// RepoFetcher fetches a repo with a particular kind
	RepoFetcher interface {
		Fetch(ctx context.Context, opts map[string]any) (fs.FS, error)
	}

	// Map is a map from kinds to repo fetchers
	Map map[string]RepoFetcher
)

func (m Map) Fetch(ctx context.Context, kind string, opts map[string]any) (fs.FS, error) {
	f, ok := m[kind]
	if !ok {
		return nil, ErrorUnknownKind
	}
	fsys, err := f.Fetch(ctx, opts)
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
	root string,
	p string,
	entry fs.DirEntry,
	hasher merkelHasher,
	filter func(p string, entry fs.DirEntry) (bool, error),
) (h2streamhash.Hash, error) {
	hash, err := hasher.Hash()
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to construct hash")
	}

	if entry.Type()&(^(fs.ModeSymlink & fs.ModeDir)) != 0 {
		// entry is not a regular file, symlink, or dir
		return nil, nil
	}

	if ok, err := filter(p, entry); err != nil {
		return nil, err
	} else if !ok {
		return nil, nil
	}

	notEmpty, err := func() (bool, error) {
		if entry.Type()&fs.ModeSymlink != 0 {
			// symlink
			dest, err := os.Readlink(filepath.Join(filepath.FromSlash(root), filepath.FromSlash(p)))
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
					log.Println(kerrors.WithMsg(err, "Failed to close open file"))
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
			h, err := merkelHash(fsys, root, path.Join(p, i.Name()), i, hasher, filter)
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
	root string,
	hasher merkelHasher,
	filter func(p string, entry fs.DirEntry) (bool, error),
) (h2streamhash.Hash, error) {
	info, err := fs.Stat(fsys, ".")
	if err != nil {
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed to read dir: %s", root))
	}
	h, err := merkelHash(fsys, root, ".", fs.FileInfoToDirEntry(info), hasher, filter)
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
	root string,
	hasher h2streamhash.Hasher,
	filter func(p string, entry fs.DirEntry) (bool, error),
) (string, error) {
	h, err := merkelTreeHash(fsys, root, hasher, filter)
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
	root string,
	verifier *h2streamhash.Verifier,
	filter func(p string, entry fs.DirEntry) (bool, error),
	checksum string,
) (bool, error) {
	h, err := merkelTreeHash(fsys, root, &verifierHasher{verifier: verifier, checksum: checksum}, filter)
	if err != nil {
		return false, err
	}
	return h.Verify(checksum)
}
