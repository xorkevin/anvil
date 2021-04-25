package component

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"
)

var (
	// ErrInvalidComponentKind is returned when attempting to parse a component with an invalid kind
	ErrInvalidComponentKind = errors.New("Invalid component kind")
)

type (
	// repoPath represents a repo component path
	repoPath struct {
		repo string
		path string
	}

	// configFileCache caches parsed component config files
	configFileCache struct {
		localfs fs.FS
		gitfs   fs.FS
		cache   map[repoPath]*ConfigFile
	}
)

func newConfigFileCache(localfs fs.FS, gitfs fs.FS) *configFileCache {
	return &configFileCache{
		localfs: localfs,
		gitfs:   gitfs,
		cache:   map[repoPath]*ConfigFile{},
	}
}

func (c *configFileCache) Parse(repo, path string) (*ConfigFile, error) {
	k := repoPath{
		repo: repo,
		path: path,
	}
	if f, ok := c.cache[k]; ok {
		return f, nil
	}
	var fsys fs.FS
	if repo == "" {
		fsys = c.localfs
	} else {
		var err error
		fsys, err = fs.Sub(c.gitfs, repo)
		if err != nil {
			return nil, fmt.Errorf("Failed to open dir %s: %w", repo, err)
		}
	}
	f, err := ParseConfigFile(fsys, path)
	if err != nil {
		return nil, err
	}
	c.cache[k] = f
	return f, nil
}

func parseComponentTreeRec(repo, path string, patch *Patch, cache *configFileCache) ([]Component, error) {
	config, err := cache.Parse(repo, path)
	if err != nil {
		return nil, err
	}
	component, err := config.Init(patch)
	if err != nil {
		return nil, err
	}
	subcomponents := make([]Component, 0, len(component.Components)+1)
	subkeys := make([]string, 0, len(component.Components))
	for k := range component.Components {
		subkeys = append(subkeys, k)
	}
	sort.Strings(subkeys)
	for _, k := range subkeys {
		sub := component.Components[k]
		var subrepo string
		switch sub.Kind {
		case componentKindLocal:
			subrepo = repo
		case componentKindGit:
			subrepo = sub.Repo
		default:
			return nil, fmt.Errorf("%w: %s", ErrInvalidComponentKind, sub.Kind)
		}
		comps, err := parseComponentTreeRec(subrepo, sub.Path, sub.Patch(), cache)
		if err != nil {
			return nil, err
		}
		subcomponents = append(subcomponents, comps...)
	}
	subcomponents = append(subcomponents, *component)
	return subcomponents, nil
}

// ParseComponentTree parses a component tree
func ParseComponentTree(localfs, gitfs fs.FS, path string, patch *Patch) ([]Component, error) {
	return parseComponentTreeRec("", path, patch, newConfigFileCache(localfs, gitfs))
}
