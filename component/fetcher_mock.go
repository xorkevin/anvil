package component

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

type (
	FetcherMock struct {
		CacheFS      fs.FS
		PathReplacer *strings.Replacer
	}
)

func NewFetcherMock(cachefs fs.FS) *FetcherMock {
	return &FetcherMock{
		CacheFS:      cachefs,
		PathReplacer: strings.NewReplacer("/", "__"),
	}
}

func (f *FetcherMock) Fetch(ctx context.Context, kind, repo, ref string) (fs.FS, error) {
	switch kind {
	case componentKindGit:
		return f.FetchGit(ctx, repo, ref)
	default:
		return nil, fmt.Errorf("%w: %s", ErrInvalidComponentKind, kind)
	}
}

func (f *FetcherMock) repoPathGit(repo, ref string) (string, error) {
	repodir := f.PathReplacer.Replace(repo)
	if !fs.ValidPath(repodir) {
		return "", fmt.Errorf("Invalid repo %s: %w", repo, fs.ErrInvalid)
	}
	if ref == "" {
		ref = gitDefaultBranch
	}
	refdir := f.PathReplacer.Replace(ref)
	if !fs.ValidPath(refdir) {
		return "", fmt.Errorf("Invalid ref %s: %w", ref, fs.ErrInvalid)
	}
	return filepath.Join("git", repodir, refdir), nil
}

func (f *FetcherMock) FetchGit(ctx context.Context, repo, ref string) (fs.FS, error) {
	repopath, err := f.repoPathGit(repo, ref)
	if err != nil {
		return nil, err
	}
	repofs, err := fs.Sub(f.CacheFS, repopath)
	if err != nil {
		return nil, fmt.Errorf("Failed to open dir %s: %w", repopath, err)
	}
	return repofs, nil
}
