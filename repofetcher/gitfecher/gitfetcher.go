package gitfetcher

import (
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

	"github.com/mitchellh/mapstructure"
	"xorkevin.dev/anvil/repofetcher"
	"xorkevin.dev/kerrors"
)

type (
	// Fetcher is a git repo fetcher
	Fetcher struct {
		fsys       fs.FS
		cacheDir   string
		GitDir     string
		GitBin     string
		Stdout     io.Writer
		Stderr     io.Writer
		NoNetwork  bool
		ForceFetch bool
		Verbose    bool
	}

	gitFetchOpts struct {
		Repo         string `mapstructure:"repo"`
		Tag          string `mapstructure:"tag"`
		Branch       string `mapstructure:"branch"`
		Commit       string `mapstructure:"commit"`
		ShallowSince string `mapstructure:"shallow_since"`
		Checksum     string `mapstructure:"checksum"`
	}
)

// New creates a new [*Fetcher] which is rooted at a particular file system
func New(cacheDir string) *Fetcher {
	return &Fetcher{
		fsys:       os.DirFS(cacheDir),
		cacheDir:   cacheDir,
		GitDir:     ".git",
		GitBin:     "git",
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		NoNetwork:  false,
		ForceFetch: false,
		Verbose:    false,
	}
}

func (f *Fetcher) repoPath(opts gitFetchOpts) (string, error) {
	var s strings.Builder
	if opts.Repo == "" {
		return "", kerrors.WithKind(nil, repofetcher.ErrorInvalidRepoSpec, "No repo specified")
	}
	s.WriteString(url.QueryEscape(opts.Repo))
	s.WriteString("@")
	if opts.Tag != "" {
		s.WriteString(url.QueryEscape(opts.Tag))
	} else if opts.Commit != "" {
		if opts.Branch == "" {
			return "", kerrors.WithKind(nil, repofetcher.ErrorInvalidRepoSpec, "Branch missing for commit")
		}
		s.WriteString(url.QueryEscape(opts.Branch))
		s.WriteString("-")
		s.WriteString(url.QueryEscape(opts.Commit))
	} else {
		return "", kerrors.WithKind(nil, repofetcher.ErrorInvalidRepoSpec, "No repo tag or commit specified")
	}
	return s.String(), nil
}

func (f *Fetcher) checkRepoDir(repodir string) (bool, error) {
	info, err := fs.Stat(f.fsys, repodir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, kerrors.WithMsg(err, "Failed to check repo")
	}
	cloned := err == nil
	if !info.IsDir() {
		return false, kerrors.WithKind(nil, repofetcher.ErrorInvalidCache, fmt.Sprintf("Cached repo is not a directory: %s", repodir))
	}
	return cloned, nil
}

func (f *Fetcher) Fetch(ctx context.Context, opts map[string]any) (fs.FS, error) {
	var fetchOpts gitFetchOpts
	if err := mapstructure.Decode(opts, &fetchOpts); err != nil {
		return nil, kerrors.WithMsg(err, "Invalid opts")
	}
	repodir, err := f.repoPath(fetchOpts)
	if err != nil {
		return nil, err
	}
	cloned, err := f.checkRepoDir(repodir)
	if err != nil {
		return nil, err
	}
	if !cloned || f.ForceFetch {
		if f.NoNetwork {
			if f.ForceFetch {
				return nil, kerrors.WithKind(nil, repofetcher.ErrorNetworkRequired, "May not force fetch without network")
			}
			return nil, kerrors.WithKind(nil, repofetcher.ErrorNetworkRequired, fmt.Sprintf("Cached repo not present: %s", repodir))
		}
		if cloned {
			if err := os.RemoveAll(filepath.Join(f.cacheDir, repodir)); err != nil {
				return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed to clean existing dir: %s", repodir))
			}
		}
		if err := f.gitClone(ctx, repodir, fetchOpts); err != nil {
			return nil, err
		}
	}
	rfsys, err := fs.Sub(f.fsys, repodir)
	if err != nil {
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed to get directory: %s", repodir))
	}
	if fetchOpts.Checksum != "" {
		// TODO check checksum if present
	}
	return rfsys, nil
}

func (f *Fetcher) gitClone(ctx context.Context, repodir string, opts gitFetchOpts) error {
	args := make([]string, 0, 8)
	args = append(args, "clone", "--single-branch")
	if opts.Commit != "" {
		args = append(args, "--branch", opts.Branch, "--no-checkout")
		if opts.ShallowSince != "" {
			args = append(args, "--shallow-since="+opts.ShallowSince)
		}
	} else {
		args = append(args, "--branch", opts.Tag, "--depth", "1")
	}
	args = append(args, opts.Repo, repodir)
	if err := f.runCmd(
		exec.CommandContext(ctx, f.GitBin, args...),
		f.cacheDir,
	); err != nil {
		return kerrors.WithMsg(err, fmt.Sprintf("Failed to clone repo: %s", opts.Repo))
	}
	if opts.Commit != "" {
		if err := f.runCmd(
			exec.CommandContext(ctx, f.GitBin, "switch", "--detach", opts.Commit),
			filepath.Join(f.cacheDir, repodir),
		); err != nil {
			return kerrors.WithMsg(err, fmt.Sprintf("Failed to checkout commit: %s", opts.Commit))
		}
	}
	return nil
}

func (f *Fetcher) runCmd(cmd *exec.Cmd, dir string) error {
	if f.Verbose {
		cmd.Stdout = f.Stdout
		cmd.Stderr = f.Stderr
	}
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}
