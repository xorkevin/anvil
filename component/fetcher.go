package component

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/mod/semver"
)

const (
	binGit = "git"
)

var (
	// ErrCacheNotGitRepo is returned when a cache file is not a git repo
	ErrCacheNotGitRepo = errors.New("Cache dir is not git repo")
	// ErrGitBinVersion is returned when a git binary is not the correct version
	ErrGitBinVersion = errors.New("Invalid git version")
	// ErrNetworkRequired is returned when a network call is required but the NoNetwork opt is enabled
	ErrNetworkRequired = errors.New("Network required")
)

type (
	Fetcher interface {
		Fetch(ctx context.Context, kind, repo, ref string) (fs.FS, error)
	}

	repoCacheKey struct {
		kind string
		repo string
	}

	repoCacheState struct {
		ref    string
		repofs fs.FS
	}

	OSFetcher struct {
		Base    string
		CacheFS fs.FS
		Opts    Opts
		cache   map[repoCacheKey]repoCacheState
		hasGit  bool
	}
)

func NewOSFetcher(base string, opts Opts) *OSFetcher {
	return &OSFetcher{
		Base:    base,
		CacheFS: os.DirFS(base),
		Opts:    opts,
		cache:   map[repoCacheKey]repoCacheState{},
	}
}

func (o *OSFetcher) Fetch(ctx context.Context, kind, repo, ref string) (fs.FS, error) {
	key := repoCacheKey{
		kind: kind,
		repo: repo,
	}
	needUpdate := true
	if s, ok := o.cache[key]; ok {
		if s.ref == ref {
			return s.repofs, nil
		}
		needUpdate = false
	}
	var repofs fs.FS
	switch kind {
	case componentKindGit:
		var err error
		repofs, err = o.FetchGit(ctx, repo, ref, needUpdate)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%w: %s", ErrInvalidComponentKind, kind)
	}
	o.cache[key] = repoCacheState{
		ref:    ref,
		repofs: repofs,
	}
	return repofs, nil
}

func (o *OSFetcher) repoPathGit(repo string) (string, error) {
	repodir := url.QueryEscape(repo)
	if !fs.ValidPath(repodir) {
		return "", fmt.Errorf("Invalid repo %s: %w", repo, fs.ErrInvalid)
	}
	return filepath.Join("git", repodir), nil
}

func (o *OSFetcher) FetchGit(ctx context.Context, repo, ref string, needUpdate bool) (fs.FS, error) {
	if !o.hasGit {
		if _, err := exec.LookPath(binGit); err != nil {
			return nil, fmt.Errorf("%s not found in PATH: %w", binGit, err)
		}
		o.hasGit = true
	}

	cacherepopath, err := o.repoPathGit(repo)
	if err != nil {
		return nil, err
	}
	repopath := filepath.Join(o.Base, cacherepopath)
	info, err := os.Stat(repopath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("Failed to check if %s already cloned: %w", repo, err)
	}
	alreadyCloned := err == nil
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s", ErrCacheNotGitRepo, repopath)
	}
	if err := o.gitVersion(ctx); err != nil {
		return nil, err
	}
	ranGitPull := false
	if needUpdate {
		if alreadyCloned {
			if !o.Opts.NoNetwork {
				if err := o.gitPull(ctx, repopath, ref); err != nil {
					return nil, err
				}
				ranGitPull = true
			}
		} else {
			if o.Opts.NoNetwork {
				return nil, fmt.Errorf("Git repo %s not present: %w", repopath, ErrNetworkRequired)
			}
			if err := o.gitClone(ctx, repopath, repo); err != nil {
				return nil, err
			}
		}
	}
	if o.gitIsBranch(ctx, repopath, ref) {
		if !ranGitPull {
			if err := o.gitSwitchBranch(ctx, repopath, ref); err != nil {
				return nil, err
			}
			if err := o.gitMerge(ctx, repopath, ref); err != nil {
				return nil, err
			}
		}
	} else {
		// is tag or commit
		if err := o.gitSwitchDetach(ctx, repopath, ref); err != nil {
			return nil, err
		}
	}
	repofs, err := fs.Sub(o.CacheFS, cacherepopath)
	if err != nil {
		return nil, fmt.Errorf("Failed to open dir %s: %w", cacherepopath, err)
	}
	return repofs, nil
}

const (
	stderrMsgCap = 4096
)

func (o *OSFetcher) runCmd(cmd *exec.Cmd) error {
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	b := &bytes.Buffer{}
	if _, err := io.CopyN(b, stderr, stderrMsgCap); err != nil {
		// error ignored as to just make a best effort attempt to copy stderr
		b.WriteString("--- truncated ---")
	}
	if _, err := io.Copy(io.Discard, stderr); err != nil {
		// error ignored as to just make a best effort attempt to discard rest of stderr
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("%w: %s", err, b.Bytes())
	}
	return nil
}

const (
	gitMinVersion    = "v2.23"
	gitVersionPrefix = "git version "
)

func (o *OSFetcher) gitVersion(ctx context.Context) error {
	b := &bytes.Buffer{}
	cmd := exec.CommandContext(ctx, binGit, "--version")
	cmd.Stdout = b
	if err := o.runCmd(cmd); err != nil {
		return fmt.Errorf("Failed to get git version: %w", err)
	}
	out := strings.TrimSpace(b.String())
	if !strings.HasPrefix(out, gitVersionPrefix) {
		return fmt.Errorf("%w: %s", ErrGitBinVersion, out)
	}
	if semver.Compare(gitMinVersion, fmt.Sprintf("v%s", strings.TrimPrefix(out, gitVersionPrefix))) > 0 {
		return fmt.Errorf("%w: %s", ErrGitBinVersion, out)
	}
	return nil
}

func (o *OSFetcher) gitClone(ctx context.Context, repopath, repo string) error {
	args := make([]string, 0, 6)
	args = append(args, "clone", "--origin", "origin")
	if o.Opts.GitPartialClone {
		args = append(args, "--filter=blob:none")
	}
	args = append(args, repo, repopath)
	if err := o.runCmd(exec.CommandContext(ctx, binGit, args...)); err != nil {
		return fmt.Errorf("Failed to git clone %s: %w", repo, err)
	}
	return nil
}

func (o *OSFetcher) gitPull(ctx context.Context, repopath, branch string) error {
	if !o.gitIsBranch(ctx, repopath, branch) {
		branch = o.gitGetDefaultBranch(ctx, repopath)
	}
	if err := o.gitSwitchBranch(ctx, repopath, branch); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, binGit, "pull", "--ff-only")
	cmd.Dir = repopath
	if err := o.runCmd(cmd); err != nil {
		return fmt.Errorf("Failed to git pull %s in %s: %w", branch, repopath, err)
	}
	return nil
}

func (o *OSFetcher) gitMerge(ctx context.Context, repopath, branch string) error {
	cmd := exec.CommandContext(ctx, binGit, "merge", "--ff-only")
	cmd.Dir = repopath
	if err := o.runCmd(cmd); err != nil {
		return fmt.Errorf("Failed to git merge for %s in %s: %w", branch, repopath, err)
	}
	return nil
}

func (o *OSFetcher) gitSwitchBranch(ctx context.Context, repopath, branch string) error {
	cmd := exec.CommandContext(ctx, binGit, "switch", branch)
	cmd.Dir = repopath
	if err := o.runCmd(cmd); err != nil {
		return fmt.Errorf("Failed to git switch to %s in %s: %w", branch, repopath, err)
	}
	return nil
}

func (o *OSFetcher) gitSwitchDetach(ctx context.Context, repopath, ref string) error {
	cmd := exec.CommandContext(ctx, binGit, "switch", "--detach", ref)
	cmd.Dir = repopath
	if err := o.runCmd(cmd); err != nil {
		return fmt.Errorf("Failed to git switch to %s in %s: %w", ref, repopath, err)
	}
	return nil
}

func (o *OSFetcher) gitIsBranch(ctx context.Context, repopath, ref string) bool {
	if ref == "" {
		return false
	}
	cmd := exec.CommandContext(ctx, binGit, "show-ref", "--quiet", "--verify", fmt.Sprintf("refs/heads/%s", ref))
	cmd.Dir = repopath
	if err := cmd.Run(); err == nil {
		return true
	}
	cmd = exec.CommandContext(ctx, binGit, "show-ref", "--quiet", "--verify", fmt.Sprintf("refs/remotes/origin/%s", ref))
	cmd.Dir = repopath
	if err := cmd.Run(); err == nil {
		return true
	}
	return false
}

func (o *OSFetcher) gitIsTag(ctx context.Context, repopath, ref string) bool {
	if ref == "" {
		return false
	}
	cmd := exec.CommandContext(ctx, binGit, "show-ref", "--quiet", "--verify", fmt.Sprintf("refs/tags/%s", ref))
	cmd.Dir = repopath
	if err := cmd.Run(); err == nil {
		return true
	}
	return false
}

const (
	gitDefaultBranch   = "master"
	gitRefOriginPrefix = "refs/remotes/origin/"
)

func (o *OSFetcher) gitGetDefaultBranch(ctx context.Context, repopath string) string {
	b := &bytes.Buffer{}
	cmd := exec.CommandContext(ctx, binGit, "symbolic-ref", "refs/remotes/origin/HEAD")
	cmd.Stdout = b
	if err := cmd.Run(); err != nil {
		return gitDefaultBranch
	}
	out := strings.TrimSpace(b.String())
	if !strings.HasPrefix(out, gitRefOriginPrefix) {
		return gitDefaultBranch
	}
	return strings.TrimPrefix(out, gitRefOriginPrefix)
}
