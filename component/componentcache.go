package component

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"path"
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
	return c.fetchers.Parse(kind, repobytes)
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
	if !fs.ValidPath(dir) {
		return nil, kerrors.WithKind(nil, ErrInvalidDir, fmt.Sprintf("Invalid repo dir: %s", dir))
	}
	repokey, err := c.repoKey(spec)
	if err != nil {
		return nil, err
	}
	cachekey := c.cacheKey(kind, repokey, dir)
	if eng, ok := c.cache[cachekey]; ok {
		return eng, nil
	}
	fsys, err := c.fetchers.Fetch(ctx, spec)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to fetch repo")
	}
	if !c.isLocalRepo(spec.Kind) {
		if sum, ok := c.checksums[repokey]; ok {
			ok, err := repofetcher.MerkelTreeVerify(fsys, c.verifier, sum)
			if err != nil {
				return nil, kerrors.WithMsg(err, "Failed verifying repo checksum")
			}
			if !ok {
				return nil, kerrors.WithKind(nil, repofetcher.ErrInvalidCache, "Repo failed integrity check")
			}
		}
		if _, ok := c.sums[repokey]; !ok {
			sum, err := repofetcher.MerkelTreeHash(fsys, c.hasher)
			if err != nil {
				return nil, kerrors.WithMsg(err, "Failed computing repo checksum")
			}
			c.sums[repokey] = sum
		}
	}
	eng, err := c.engines.Build(kind, fsys)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to build config engine")
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
