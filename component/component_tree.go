package component

import (
	"context"
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
	// configFileCache caches parsed component config files
	configFileCache struct {
		localfs  fs.FS
		remotefs fs.FS
		fetcher  Fetcher
		cache    map[RepoPath]*ConfigFile
	}
)

func newConfigFileCache(localfs, remotefs fs.FS, fetcher Fetcher) *configFileCache {
	return &configFileCache{
		localfs:  localfs,
		remotefs: remotefs,
		fetcher:  fetcher,
		cache:    map[RepoPath]*ConfigFile{},
	}
}

func (c *configFileCache) Parse(ctx context.Context, path RepoPath) (*ConfigFile, error) {
	if f, ok := c.cache[path]; ok {
		return f, nil
	}
	var fsys fs.FS
	if path.Repo == "" {
		fsys = c.localfs
	} else {
		if err := c.fetcher.Fetch(ctx, path.Kind, path.Repo, path.Ref); err != nil {
			return nil, err
		}
		var err error
		fsys, err = fs.Sub(c.remotefs, path.Repo)
		if err != nil {
			return nil, fmt.Errorf("Failed to open dir %s: %w", path.Repo, err)
		}
	}
	f, err := ParseConfigFile(fsys, path.Path)
	if err != nil {
		return nil, err
	}
	c.cache[path] = f
	return f, nil
}

func parseComponentTreeRec(ctx context.Context, path RepoPath, patch *Patch, parents []RepoPath, cache *configFileCache) ([]Component, error) {
	for _, i := range parents {
		if path == i {
			return nil, fmt.Errorf("%w: %s -> %s", ErrImportCycle, parents[len(parents)-1], path)
		}
	}
	parents = append(parents, path)

	config, err := cache.Parse(ctx, path)
	if err != nil {
		return nil, err
	}
	component, deps, err := config.Init(patch)
	if err != nil {
		return nil, err
	}

	components := make([]Component, 0, len(deps)+1)
	for _, i := range deps {
		subpath := i.Path
		switch i.Path.Kind {
		case componentKindLocal:
			subpath.Repo = path.Repo
		case componentKindGit:
			subpath.Repo = i.Path.Repo
		default:
			return nil, fmt.Errorf("%w: %s", ErrInvalidComponentKind, i.Path.Kind)
		}
		children, err := parseComponentTreeRec(ctx, subpath, i.Patch(), parents, cache)
		if err != nil {
			return nil, err
		}
		components = append(components, children...)
	}
	components = append(components, *component)
	return components, nil
}

// ParseComponentTree parses a component tree
func ParseComponentTree(ctx context.Context, localfs, remotefs fs.FS, fetcher Fetcher, path string, patch *Patch) ([]Component, error) {
	return parseComponentTreeRec(ctx, RepoPath{
		Repo: "",
		Path: path,
	}, patch, nil, newConfigFileCache(localfs, remotefs, fetcher))
}

// ParseComponents parses components
func ParseComponents(ctx context.Context, outputfs WriteFS, localfs, remotefs fs.FS, fetcher Fetcher, path, patchpath string) ([]Component, error) {
	var patch *Patch
	if patchpath != "" {
		var err error
		patch, err = ParsePatchFile(localfs, patchpath)
		if err != nil {
			return nil, err
		}
	}
	components, err := ParseComponentTree(ctx, localfs, remotefs, fetcher, path, patch)
	if err != nil {
		return nil, err
	}
	return components, nil
}

// WriteComponents writes components to an output fs
func WriteComponents(outputfs WriteFS, components []Component) error {
	for _, i := range components {
		if err := i.Generate(outputfs); err != nil {
			return err
		}
	}
	return nil
}

// GenerateComponents generates components
func GenerateComponents(ctx context.Context, outputfs WriteFS, localfs, remotefs fs.FS, fetcher Fetcher, path, patchpath string) error {
	components, err := ParseComponents(ctx, outputfs, localfs, remotefs, fetcher, path, patchpath)
	if err != nil {
		return err
	}
	if err := WriteComponents(outputfs, components); err != nil {
		return err
	}
	return nil
}

type (
	// Opts holds generation opts
	Opts struct {
		GitPartialClone bool
	}
)

// Generate reads configs and writes components to the filesystem
func Generate(ctx context.Context, output, local, remote, path, patchpath string, opts Opts) error {
	outputfs := NewOSWriteFS(output)
	localfs := os.DirFS(local)
	remotefs := os.DirFS(remote)
	fetcher := NewOSFetcher(remote, opts)
	var err error
	path, err = filepath.Rel(local, path)
	if err != nil {
		return fmt.Errorf("Failed to construct relative path: %w", err)
	}
	if patchpath != "" {
		var err error
		patchpath, err = filepath.Rel(local, patchpath)
		if err != nil {
			return fmt.Errorf("Failed to construct relative path: %w", err)
		}
	}
	if err := GenerateComponents(ctx, outputfs, localfs, remotefs, fetcher, path, patchpath); err != nil {
		return err
	}
	return nil
}
