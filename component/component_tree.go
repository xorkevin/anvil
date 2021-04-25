package component

import (
	"errors"
	"fmt"
	"io/fs"
)

var (
	// ErrInvalidComponentKind is returned when attempting to parse a component with an invalid kind
	ErrInvalidComponentKind = errors.New("Invalid component kind")
	// ErrImportCycle is returned when component dependencies form a cycle
	ErrImportCycle = errors.New("Import cycle")
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

func (r repoPath) String() string {
	return fmt.Sprintf("%s:%s", r.repo, r.path)
}

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

func parseComponentTreeRec(repo, path string, patch *Patch, parents []repoPath, cache *configFileCache) ([]Component, error) {
	current := repoPath{
		repo: repo,
		path: path,
	}
	for _, i := range parents {
		if current == i {
			return nil, fmt.Errorf("%w: %s -> %s", ErrImportCycle, parents[len(parents)-1], current)
		}
	}
	parents = append(parents, current)

	config, err := cache.Parse(repo, path)
	if err != nil {
		return nil, err
	}
	component, deps, err := config.Init(patch)
	if err != nil {
		return nil, err
	}

	components := make([]Component, 0, len(deps)+1)
	for _, i := range deps {
		var subrepo string
		switch i.Kind {
		case componentKindLocal:
			subrepo = repo
		case componentKindGit:
			subrepo = i.Repo
		default:
			return nil, fmt.Errorf("%w: %s", ErrInvalidComponentKind, i.Kind)
		}
		children, err := parseComponentTreeRec(subrepo, i.Path, i.Patch(), parents, cache)
		if err != nil {
			return nil, err
		}
		components = append(components, children...)
	}
	components = append(components, *component)
	return components, nil
}

// ParseComponentTree parses a component tree
func ParseComponentTree(localfs, gitfs fs.FS, path string, patch *Patch) ([]Component, error) {
	return parseComponentTreeRec("", path, patch, nil, newConfigFileCache(localfs, gitfs))
}
