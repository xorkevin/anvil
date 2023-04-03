package component

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"sort"
	"strings"

	"xorkevin.dev/anvil/confengine"
	"xorkevin.dev/anvil/repofetcher"
	"xorkevin.dev/hunter2/h2streamhash"
	"xorkevin.dev/hunter2/h2streamhash/blake2bstream"
	"xorkevin.dev/kerrors"
)

// ErrInvalidDir is returned when the repo dir is invalid
var ErrInvalidDir errInvalidDir

type (
	errInvalidDir struct{}
)

func (e errInvalidDir) Error() string {
	return "Invalid repo dir"
}

type (
	// Cache is a config engine cache by path
	Cache struct {
		fetchers  repofetcher.Map
		engines   confengine.Map
		cache     map[string]confengine.ConfEngine
		local     map[string]struct{}
		checksums map[string]string
		hasher    h2streamhash.Hasher
		verifier  *h2streamhash.Verifier
		sums      map[string]string
	}

	// RepoChecksum is a checksum for a repo
	RepoChecksum struct {
		Key string
		Sum string
	}
)

// NewCache creates a new [*Cache]
func NewCache(fetchers repofetcher.Map, engines confengine.Map, local map[string]struct{}, checksums map[string]string) *Cache {
	hasher := blake2bstream.NewHasher(blake2bstream.Config{})
	verifier := h2streamhash.NewVerifier()
	verifier.Register(hasher)

	return &Cache{
		fetchers:  fetchers,
		engines:   engines,
		cache:     map[string]confengine.ConfEngine{},
		local:     local,
		checksums: checksums,
		hasher:    hasher,
		verifier:  verifier,
		sums:      map[string]string{},
	}
}

func (c *Cache) Parse(kind string, repobytes []byte) (repofetcher.Spec, error) {
	spec, err := c.fetchers.Parse(kind, repobytes)
	if err != nil {
		return repofetcher.Spec{}, kerrors.WithMsg(err, "Failed to parse repo spec")
	}
	return spec, nil
}

func (c *Cache) repoKey(spec repofetcher.Spec) (string, error) {
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

func (c *Cache) cacheKey(kind string, repokey string, dir string) string {
	var s strings.Builder
	s.WriteString(url.QueryEscape(kind))
	s.WriteString(":")
	s.WriteString(repokey)
	s.WriteString(":")
	s.WriteString(dir)
	return s.String()
}

func (c *Cache) isLocalRepo(repokind string) bool {
	_, ok := c.local[repokind]
	return ok
}

func (c *Cache) Get(ctx context.Context, kind string, spec repofetcher.Spec, dir string) (confengine.ConfEngine, error) {
	repokey, err := c.repoKey(spec)
	if err != nil {
		return nil, err
	}
	if !fs.ValidPath(dir) {
		return nil, kerrors.WithKind(nil, ErrInvalidDir, fmt.Sprintf("Invalid repo dir %s for repo %s", dir, repokey))
	}
	cachekey := c.cacheKey(kind, repokey, dir)
	if eng, ok := c.cache[cachekey]; ok {
		return eng, nil
	}
	fsys, err := c.fetchers.Fetch(ctx, spec)
	if err != nil {
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed to fetch repo for repo: %s", repokey))
	}
	if !c.isLocalRepo(spec.Kind) {
		if sum, ok := c.checksums[repokey]; ok {
			ok, err := repofetcher.MerkelTreeVerify(fsys, c.verifier, sum)
			if err != nil {
				return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed verifying checksum for repo: %s", repokey))
			}
			if !ok {
				return nil, kerrors.WithKind(nil, repofetcher.ErrInvalidCache, fmt.Sprintf("Failed integrity check for repo: %s", repokey))
			}
		}
		if _, ok := c.sums[repokey]; !ok {
			sum, err := repofetcher.MerkelTreeHash(fsys, c.hasher)
			if err != nil {
				return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed computing checksum for repo: %s", repokey))
			}
			c.sums[repokey] = sum
		}
	}
	fsys, err = fs.Sub(fsys, dir)
	if err != nil {
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed to get subdirectory %s for repo %s", dir, repokey))
	}
	eng, err := c.engines.Build(kind, fsys)
	if err != nil {
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed to build %s config engine for repo %s at dir %s", kind, repokey, dir))
	}
	c.cache[cachekey] = eng
	return eng, nil
}

func (c *Cache) Sums() []RepoChecksum {
	keys := make([]string, 0, len(c.sums))
	for k := range c.sums {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	sums := make([]RepoChecksum, 0, len(keys))
	for _, i := range keys {
		sums = append(sums, RepoChecksum{
			Key: i,
			Sum: c.sums[i],
		})
	}
	return sums
}
