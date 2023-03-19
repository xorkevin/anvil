package repofetcher

import (
	"context"
	"io/fs"

	"xorkevin.dev/kerrors"
)

var (
	// ErrorUnknownKind is returned when the repo kind is not supported
	ErrorUnknownKind errUnknownKind
	// ErrorInvalidRepoSpec is returned when the repo spec is invalid
	ErrorInvalidRepoSpec errInvalidRepoSpec
	// ErrorInvalidCache is returned when the cached repo is invalid
	ErrorInvalidCache errInvalidCache
	// ErrorNetworkRequired is returned when the network is required to complete the operation
	ErrorNetworkRequired errNetworkRequired
)

type (
	errUnknownKind     struct{}
	errInvalidRepoSpec struct{}
	errInvalidCache    struct{}
	errNetworkRequired struct{}
)

func (e errUnknownKind) Error() string {
	return "Unknown repo kind"
}

func (e errInvalidRepoSpec) Error() string {
	return "Invalid repo spec"
}

func (e errInvalidCache) Error() string {
	return "Invalid cache"
}

func (e errNetworkRequired) Error() string {
	return "Network required"
}

type (
	// RepoFetcher fetches a repo with a particular kind
	RepoFetcher interface {
		Fetch(ctx context.Context, opts map[string]any) (fs.FS, error)
	}

	// Map is a map from kinds to repo fetchers
	Map map[string]RepoFetcher
)

func (m Map) Fetch(ctx context.Context, kind string, opts map[string]any) (fs.FS, error) {
	f, ok := m[kind]
	if !ok {
		return nil, ErrorUnknownKind
	}
	fsys, err := f.Fetch(ctx, opts)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to fetch repo")
	}
	return fsys, nil
}
