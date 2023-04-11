package component

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"strings"

	"xorkevin.dev/anvil/confengine"
	"xorkevin.dev/anvil/repofetcher"
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
		repos   *repofetcher.Cache
		engines confengine.Map
		cache   map[string]confengine.ConfEngine
	}
)

// NewCache creates a new [*Cache]
func NewCache(repos *repofetcher.Cache, engines confengine.Map) *Cache {
	return &Cache{
		repos:   repos,
		engines: engines,
		cache:   map[string]confengine.ConfEngine{},
	}
}

func (c *Cache) Parse(kind string, repobytes []byte) (repofetcher.Spec, error) {
	spec, err := c.repos.Parse(kind, repobytes)
	if err != nil {
		return repofetcher.Spec{}, kerrors.WithMsg(err, "Failed to parse repo spec")
	}
	return spec, nil
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

func (c *Cache) Get(ctx context.Context, kind string, spec repofetcher.Spec, dir string) (confengine.ConfEngine, error) {
	fsys, err := c.repos.Get(ctx, spec)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to fetch repo")
	}
	repokey := spec.String()
	if !fs.ValidPath(dir) {
		return nil, kerrors.WithKind(nil, ErrInvalidDir, fmt.Sprintf("Invalid repo dir %s for repo %s", dir, repokey))
	}
	cachekey := c.cacheKey(kind, repokey, dir)
	if eng, ok := c.cache[cachekey]; ok {
		return eng, nil
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
