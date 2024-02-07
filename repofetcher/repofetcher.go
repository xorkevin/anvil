package repofetcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"path"
	"slices"
	"strings"

	"xorkevin.dev/hunter2/h2streamhash"
	"xorkevin.dev/hunter2/h2streamhash/blake2bstream"
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

func (s Spec) String() string {
	speckey, err := s.RepoSpec.Key()
	if err != nil {
		speckey = "error"
	}
	var b strings.Builder
	b.WriteString(url.QueryEscape(s.Kind))
	b.WriteString(":")
	b.WriteString(speckey)
	return b.String()
}

func (m Map) Parse(kind string, repobytes []byte) (Spec, error) {
	f, ok := m[kind]
	if !ok {
		return Spec{}, kerrors.WithKind(nil, ErrUnknownKind, fmt.Sprintf("Unknown repo kind: %s", kind))
	}
	repospec, err := f.Parse(repobytes)
	if err != nil {
		return Spec{}, kerrors.WithMsg(err, fmt.Sprintf("Failed to build %s repo spec", kind))
	}
	if _, err := repospec.Key(); err != nil {
		return Spec{}, kerrors.WithMsg(err, fmt.Sprintf("Invalid %s repo spec", kind))
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
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed to fetch %s repo", spec.Kind))
	}
	return fsys, nil
}

type (
	// Cache is a repo fetcher that caches results
	Cache struct {
		fetchers  Map
		cache     map[string]fs.FS
		local     map[string]struct{}
		checksums map[string]string
		hasher    h2streamhash.Hasher
		verifier  *h2streamhash.Verifier
		sums      map[string]string
	}

	// RepoChecksum is a checksum for a repo
	RepoChecksum struct {
		Key string `json:"key"`
		Sum string `json:"sum"`
	}
)

func NewCache(fetchers Map, local map[string]struct{}, checksums map[string]string) *Cache {
	hasher := blake2bstream.NewHasher(blake2bstream.Config{})
	verifier := h2streamhash.NewVerifier()
	verifier.Register(hasher)
	return &Cache{
		fetchers:  fetchers,
		cache:     map[string]fs.FS{},
		local:     local,
		checksums: checksums,
		hasher:    hasher,
		verifier:  verifier,
		sums:      map[string]string{},
	}
}

func (c *Cache) Parse(kind string, repobytes []byte) (Spec, error) {
	spec, err := c.fetchers.Parse(kind, repobytes)
	if err != nil {
		return Spec{}, kerrors.WithMsg(err, "Failed to parse repo spec")
	}
	return spec, nil
}

func (c *Cache) repoKey(spec Spec) (string, error) {
	speckey, err := spec.RepoSpec.Key()
	if err != nil {
		return "", kerrors.WithMsg(err, "Failed to compute repo spec key")
	}
	var s strings.Builder
	s.WriteString(url.QueryEscape(spec.Kind))
	s.WriteString(":")
	s.WriteString(speckey)
	return s.String(), nil
}

func (c *Cache) isLocalRepo(repokind string) bool {
	_, ok := c.local[repokind]
	return ok
}

func (c *Cache) Get(ctx context.Context, spec Spec) (fs.FS, error) {
	repokey, err := c.repoKey(spec)
	if err != nil {
		return nil, err
	}
	if fsys, ok := c.cache[repokey]; ok {
		return fsys, nil
	}
	fsys, err := c.fetchers.Fetch(ctx, spec)
	if err != nil {
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed to fetch repo for repo: %s", repokey))
	}
	if !c.isLocalRepo(spec.Kind) {
		if sum, ok := c.checksums[repokey]; ok {
			ok, err := MerkelTreeVerify(fsys, c.verifier, sum)
			if err != nil {
				return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed verifying checksum for repo: %s", repokey))
			}
			if !ok {
				return nil, kerrors.WithKind(nil, ErrInvalidCache, fmt.Sprintf("Failed integrity check for repo: %s", repokey))
			}
		}
		if _, ok := c.sums[repokey]; !ok {
			sum, err := MerkelTreeHash(fsys, c.hasher)
			if err != nil {
				return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed computing checksum for repo: %s", repokey))
			}
			c.sums[repokey] = sum
		}
	}
	c.cache[repokey] = fsys
	return fsys, nil
}

func (c *Cache) Sums() []RepoChecksum {
	keys := make([]string, 0, len(c.sums))
	for k := range c.sums {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	sums := make([]RepoChecksum, 0, len(keys))
	for _, i := range keys {
		sums = append(sums, RepoChecksum{
			Key: i,
			Sum: c.sums[i],
		})
	}
	return sums
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
				return false, kerrors.WithMsg(err, fmt.Sprintf("Failed to read symlink %s", p))
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
