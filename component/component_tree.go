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
