package component

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
		localfs  fs.FS
		remotefs fs.FS
		cache    map[repoPath]*ConfigFile
	}
)

func (r repoPath) String() string {
	return fmt.Sprintf("%s:%s", r.repo, r.path)
}

func newConfigFileCache(localfs fs.FS, remotefs fs.FS) *configFileCache {
	return &configFileCache{
		localfs:  localfs,
		remotefs: remotefs,
		cache:    map[repoPath]*ConfigFile{},
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
		fsys, err = fs.Sub(c.remotefs, repo)
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
func ParseComponentTree(localfs, remotefs fs.FS, path string, patch *Patch) ([]Component, error) {
	return parseComponentTreeRec("", path, patch, nil, newConfigFileCache(localfs, remotefs))
}

// GenerateComponents generates components and writes them to an output fs
func GenerateComponents(outputfs WriteFS, localfs, remotefs fs.FS, path, patchpath string) error {
	var patch *Patch
	if patchpath != "" {
		var err error
		patch, err = ParsePatchFile(localfs, patchpath)
		if err != nil {
			return err
		}
	}
	components, err := ParseComponentTree(localfs, remotefs, path, patch)
	if err != nil {
		return err
	}
	for _, i := range components {
		if err := i.Generate(outputfs); err != nil {
			return err
		}
	}
	return nil
}

// Generate reads configs and writes components to the filesystem
func Generate(output, local, remote, path, patchpath string) error {
	outputfs := NewOSWriteFS(output)
	localfs := os.DirFS(local)
	remotefs := os.DirFS(remote)
	var err error
	path, err = filepath.Rel(local, path)
	if err != nil {
		return fmt.Errorf("Failed to construct relative path: %w", err)
	}
	if patchpath != "" {
		patchpath, err = filepath.Rel(local, patchpath)
		if err != nil {
			return fmt.Errorf("Failed to construct relative path: %w", err)
		}
	}
	return GenerateComponents(outputfs, localfs, remotefs, path, patchpath)
}
