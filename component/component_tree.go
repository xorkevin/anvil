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
		localfs fs.FS
		fetcher Fetcher
		cache   map[RepoPath]*ConfigFile
	}
)

func newConfigFileCache(localfs fs.FS, fetcher Fetcher) *configFileCache {
	return &configFileCache{
		localfs: localfs,
		fetcher: fetcher,
		cache:   map[RepoPath]*ConfigFile{},
	}
}

func (c *configFileCache) Parse(ctx context.Context, src RepoPath) (*ConfigFile, error) {
	if f, ok := c.cache[src]; ok {
		return f, nil
	}
	var repofs fs.FS
	if src.Repo == "" {
		repofs = c.localfs
	} else {
		var err error
		repofs, err = c.fetcher.Fetch(ctx, src.Kind, src.Repo, src.Ref)
		if err != nil {
			return nil, err
		}
	}
	f, err := ParseConfigFile(repofs, src.Path)
	if err != nil {
		return nil, err
	}
	c.cache[src] = f
	return f, nil
}

func parseComponentTreeRec(ctx context.Context, src RepoPath, patch *Patch, parents []RepoPath, cache *configFileCache) ([]Component, error) {
	for _, i := range parents {
		if src == i {
			return nil, fmt.Errorf("%w: %s -> %s", ErrImportCycle, parents[len(parents)-1], src)
		}
	}
	parents = append(parents, src)

	config, err := cache.Parse(ctx, src)
	if err != nil {
		return nil, fmt.Errorf("Error parsing %s: %w", src, err)
	}
	component, deps, err := config.Init(patch)
	if err != nil {
		return nil, fmt.Errorf("Error initing %s: %w", src, err)
	}

	components := make([]Component, 0, len(deps)+1)
	for _, i := range deps {
		subsrc := i.Src
		switch i.Src.Kind {
		case componentKindLocal:
			subsrc.Repo = src.Repo
		case componentKindGit:
			subsrc.Repo = i.Src.Repo
		default:
			return nil, fmt.Errorf("%w %s: %s", ErrInvalidComponentKind, src, i.Src.Kind)
		}
		children, err := parseComponentTreeRec(ctx, subsrc, i.Patch(), parents, cache)
		if err != nil {
			return nil, fmt.Errorf("Error in subcomponent of %s: %w", src, err)
		}
		components = append(components, children...)
	}
	components = append(components, *component)
	return components, nil
}

// ParseComponentTree parses a component tree
func ParseComponentTree(ctx context.Context, localfs fs.FS, fetcher Fetcher, path string, patch *Patch) ([]Component, error) {
	return parseComponentTreeRec(ctx, RepoPath{
		Repo: "",
		Path: path,
	}, patch, nil, newConfigFileCache(localfs, fetcher))
}

// ParseComponents parses components
func ParseComponents(ctx context.Context, outputfs WriteFS, localfs fs.FS, fetcher Fetcher, path, patchpath string) ([]Component, error) {
	var patch *Patch
	if patchpath != "" {
		var err error
		patch, err = ParsePatchFile(localfs, patchpath)
		if err != nil {
			return nil, err
		}
	}
	components, err := ParseComponentTree(ctx, localfs, fetcher, path, patch)
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
func GenerateComponents(ctx context.Context, outputfs WriteFS, localfs fs.FS, fetcher Fetcher, path, patchpath string) error {
	components, err := ParseComponents(ctx, outputfs, localfs, fetcher, path, patchpath)
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
		NoNetwork       bool
		GitPartialClone bool
	}
)

// Generate reads configs and writes components to the filesystem
func Generate(ctx context.Context, output, local, remote, path, patchpath string, opts Opts) error {
	outputfs := NewOSWriteFS(output)
	localfs := os.DirFS(local)
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
	if err := GenerateComponents(ctx, outputfs, localfs, fetcher, path, patchpath); err != nil {
		return err
	}
	return nil
}
