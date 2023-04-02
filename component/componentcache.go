package component

import (
	"context"
	"net/url"
	"sort"
	"strings"

	"xorkevin.dev/anvil/confengine"
	"xorkevin.dev/anvil/repofetcher"
	"xorkevin.dev/hunter2/h2streamhash"
	"xorkevin.dev/hunter2/h2streamhash/blake2bstream"
	"xorkevin.dev/kerrors"
)

type (
	// Cache is a config engine cache by path
	Cache struct {
		fetchers  repofetcher.Map
		engines   confengine.Map
		cache     map[string]confengine.ConfEngine
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
func NewCache(checksums map[string]string, fetchers repofetcher.Map, engines confengine.Map) *Cache {
	hasher := blake2bstream.NewHasher(blake2bstream.Config{})
	verifier := h2streamhash.NewVerifier()
	verifier.Register(hasher)

	return &Cache{
		fetchers:  fetchers,
		engines:   engines,
		cache:     map[string]confengine.ConfEngine{},
		checksums: checksums,
		hasher:    hasher,
		verifier:  verifier,
		sums:      map[string]string{},
	}
}

func (c *Cache) Parse(kind string, repobytes []byte) (repofetcher.Spec, error) {
	return c.fetchers.Parse(kind, repobytes)
}

func (c *Cache) repoKey(repokind string, key string) string {
	var s strings.Builder
	s.WriteString(url.QueryEscape(repokind))
	s.WriteString(":")
	s.WriteString(key)
	return s.String()
}

func (c *Cache) cacheKey(kind string, repokind string, key string) string {
	var s strings.Builder
	s.WriteString(url.QueryEscape(kind))
	s.WriteString(":")
	s.WriteString(url.QueryEscape(repokind))
	s.WriteString(":")
	s.WriteString(key)
	return s.String()
}

func (c *Cache) Get(ctx context.Context, kind string, spec repofetcher.Spec) (confengine.ConfEngine, error) {
	speckey, err := spec.RepoSpec.Key()
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to compute repo spec key")
	}
	key := c.cacheKey(kind, spec.Kind, speckey)
	if eng, ok := c.cache[key]; ok {
		return eng, nil
	}
	fsys, err := c.fetchers.Fetch(ctx, spec)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to fetch repo")
	}
	repokey := c.repoKey(spec.Kind, speckey)
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
	eng, err := c.engines.Build(kind, fsys)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to build config engine")
	}
	c.cache[key] = eng
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
