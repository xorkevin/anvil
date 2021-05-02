package component

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	binGit = "git"
)

const (
	stderrMsgCap = 4096
)

var (
	// ErrCacheNotGitRepo is returned when a cache file is not a git repo
	ErrCacheNotGitRepo = errors.New("Cache dir is not git repo")
)

type (
	Fetcher interface {
		Fetch(ctx context.Context, kind, repo, ref string) error
	}

	OSFetcher struct {
		Base         string
		PathReplacer *strings.Replacer
		Opts         Opts
	}
)

func NewOSFetcher(base string, opts Opts) *OSFetcher {
	return &OSFetcher{
		Base:         base,
		PathReplacer: strings.NewReplacer("/", "_"),
		Opts:         opts,
	}
}

func (o *OSFetcher) Fetch(ctx context.Context, kind, repo, ref string) error {
	switch kind {
	case componentKindGit:
	}
	return nil
}

func (o *OSFetcher) FetchGit(ctx context.Context, repo, ref string) error {
	if _, err := exec.LookPath(binGit); err != nil {
		return fmt.Errorf("%s not found in PATH: %w", binGit, err)
	}
	repodir := o.PathReplacer.Replace(repo)
	if fs.ValidPath(repodir) {
		return fmt.Errorf("Invalid repo %s: %w", repo, fs.ErrInvalid)
	}
	repopath := filepath.Join(o.Base, "git", repodir)
	info, err := os.Stat(repopath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("Failed to check if %s already cloned: %w", repo, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: %s", ErrCacheNotGitRepo, repopath)
	}
	if err == nil {
		if err := o.gitPull(ctx, repopath, repo); err != nil {
			return err
		}
	} else {
		if err := o.gitClone(ctx, repopath, repo); err != nil {
			return err
		}
	}
	return nil
}

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

func (o *OSFetcher) gitPull(ctx context.Context, repopath, ref string) error {
	branch := ref
	if !o.gitIsBranch(ctx, repopath, ref) {
		b := &bytes.Buffer{}
		cmd := exec.CommandContext(ctx, binGit, "symbolic-ref", "refs/remotes/origin/HEAD")
		cmd.Stdout = b
		if err := cmd.Run(); err == nil {
			out := strings.TrimSpace(b.String())
			branch = strings.TrimPrefix(out, "refs/remotes/origin/")
			if branch == out {
				branch = "master"
			}
		} else {
			branch = "master"
		}
	}
	cmd := exec.CommandContext(ctx, binGit, "switch", branch)
	cmd.Dir = repopath
	if err := o.runCmd(cmd); err != nil {
		return fmt.Errorf("Failed to git switch to %s in %s: %w", branch, repopath, err)
	}
	cmd = exec.CommandContext(ctx, binGit, "pull", "--ff-only")
	cmd.Dir = repopath
	if err := o.runCmd(cmd); err != nil {
		return fmt.Errorf("Failed to git pull %s in %s: %w", branch, repopath, err)
	}
	return nil
}

func (o *OSFetcher) gitIsBranch(ctx context.Context, repopath, ref string) bool {
	if ref == "" {
		return false
	}
	cmd := exec.CommandContext(ctx, binGit, "show-ref", "--verify", fmt.Sprintf("refs/heads/%s", ref))
	cmd.Dir = repopath
	if err := cmd.Run(); err == nil {
		return true
	}
	cmd = exec.CommandContext(ctx, binGit, "show-ref", "--verify", fmt.Sprintf("refs/remotes/origin/%s", ref))
	cmd.Dir = repopath
	if err := cmd.Run(); err == nil {
		return true
	}
	return false
}
