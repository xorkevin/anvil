package repofetcher

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path"

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

func merkelHash(fsys fs.FS, root string, p string, entry fs.DirEntry, hasher h2streamhash.Hasher, filter func(root string, p string, entry fs.DirEntry) (bool, error)) (string, bool, error) {
	hash, err := hasher.Hash()
	if err != nil {
		return "", false, kerrors.WithMsg(err, "Failed to construct hash")
	}

	if entry.Type()&(^(fs.ModeSymlink & fs.ModeDir)) != 0 {
		// entry is not a regular file, symlink, or dir
		return "", false, nil
	}

	if ok, err := filter(root, p, entry); err != nil {
		return "", false, err
	} else if !ok {
		return "", false, nil
	}

	if err := func() error {
		if entry.Type()&fs.ModeSymlink != 0 {
			// symlink
			dest, err := os.Readlink(path.Join(root, p))
			if err != nil {
				return kerrors.WithMsg(err, fmt.Sprintf("Failed to read symlink: %s", path.Join(root, p)))
			}
			if _, err := io.WriteString(hash, dest); err != nil {
				return kerrors.WithMsg(err, "Failed to write to hash")
			}
			return nil
		} else if !entry.IsDir() {
			// regular file
			f, err := fsys.Open(p)
			if err != nil {
				return kerrors.WithMsg(err, fmt.Sprintf("Failed to open file: %s", path.Join(root, p)))
			}
			defer func() {
				if err := f.Close(); err != nil {
					log.Println(kerrors.WithMsg(err, "Failed to close open file"))
				}
			}()
			if _, err := io.Copy(hash, f); err != nil {
				return kerrors.WithMsg(err, fmt.Sprintf("Failed reading file: %s", path.Join(root, p)))
			}
			return nil
		}
		// directory
		entries, err := fs.ReadDir(fsys, p)
		if err != nil {
			return kerrors.WithMsg(err, fmt.Sprintf("Failed reading dir: %s", path.Join(root, p)))
		}
		for _, i := range entries {
			h, ok, err := merkelHash(fsys, root, path.Join(p, i.Name()), i, hasher, filter)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			if _, err := io.WriteString(hash, i.Name()); err != nil {
				return kerrors.WithMsg(err, "Failed to write to hash")
			}
			if _, err := hash.Write([]byte{0}); err != nil {
				return kerrors.WithMsg(err, "Failed to write to hash")
			}
			if _, err := io.WriteString(hash, h); err != nil {
				return kerrors.WithMsg(err, "Failed to write to hash")
			}
			if _, err := hash.Write([]byte{0}); err != nil {
				return kerrors.WithMsg(err, "Failed to write to hash")
			}
		}
		return nil
	}(); err != nil {
		return "", false, err
	}

	if err := hash.Close(); err != nil {
		return "", false, kerrors.WithMsg(err, "Failed to close hash")
	}
	return hash.Sum(), true, nil
}

func MerkelTreeHash(fsys fs.FS, root string, hasher h2streamhash.Hasher, filter func(root string, p string, entry fs.DirEntry) (bool, error)) (string, bool, error) {
	info, err := fs.Stat(fsys, ".")
	if err != nil {
		return "", false, kerrors.WithMsg(err, fmt.Sprintf("Failed to read file: %s", root))
	}
	return merkelHash(fsys, root, ".", fs.FileInfoToDirEntry(info), hasher, filter)
}
