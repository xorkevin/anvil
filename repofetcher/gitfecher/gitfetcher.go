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
	"path"
	"path/filepath"
	"strings"

	"xorkevin.dev/anvil/repofetcher"
	"xorkevin.dev/anvil/util/kjson"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/kfs"
)

type (
	// Fetcher is a git repo fetcher
	Fetcher struct {
		fsys         fs.FS
		cacheDir     string
		gitDir       string
		gitDirPrefix string
		GitCmd       GitCmd
		NoNetwork    bool
		ForceFetch   bool
	}

	// RepoSpec are git fetch opts
	RepoSpec struct {
		Repo         string `json:"repo"`
		Tag          string `json:"tag"`
		Branch       string `json:"branch"`
		Commit       string `json:"commit"`
		ShallowSince string `json:"shallow_since"`
	}

	GitCmd interface {
		GitClone(ctx context.Context, repodir string, repospec RepoSpec) error
	}

	// Opt is a constructor option
	Opt = func(*Fetcher)
)

// New creates a new git [*Fetcher] which is rooted at a particular file system
func New(cacheDir string, opts ...Opt) *Fetcher {
	f := &Fetcher{
		fsys:         os.DirFS(filepath.FromSlash(cacheDir)),
		cacheDir:     cacheDir,
		gitDir:       ".git",
		gitDirPrefix: ".git/",
		GitCmd:       NewGitBin(cacheDir),
		NoNetwork:    false,
		ForceFetch:   false,
	}
	for _, i := range opts {
		i(f)
	}
	return f
}

func OptGitDir(p string) Opt {
	return func(f *Fetcher) {
		f.gitDir = path.Clean(p)
		f.gitDirPrefix = f.gitDir + "/"
	}
}

func (o RepoSpec) Key() (string, error) {
	var s strings.Builder
	if o.Repo == "" {
		return "", kerrors.WithKind(nil, repofetcher.ErrInvalidRepoSpec, "No repo specified")
	}
	s.WriteString(url.QueryEscape(o.Repo))
	s.WriteString("@")
	if o.Tag != "" {
		s.WriteString(url.QueryEscape(o.Tag))
	} else if o.Commit != "" {
		if o.Branch == "" {
			return "", kerrors.WithKind(nil, repofetcher.ErrInvalidRepoSpec, "Branch missing for commit")
		}
		s.WriteString(url.QueryEscape(o.Branch))
		s.WriteString("-")
		s.WriteString(url.QueryEscape(o.Commit))
	} else {
		return "", kerrors.WithKind(nil, repofetcher.ErrInvalidRepoSpec, "No repo tag or commit specified")
	}
	return s.String(), nil
}

func (f *Fetcher) Parse(specbytes []byte) (repofetcher.RepoSpec, error) {
	var repospec RepoSpec
	if err := kjson.Unmarshal(specbytes, &repospec); err != nil {
		return nil, kerrors.WithKind(err, repofetcher.ErrInvalidRepoSpec, "Failed to parse spec bytes")
	}
	return repospec, nil
}

func (f *Fetcher) checkRepoDir(repodir string) (bool, error) {
	info, err := fs.Stat(f.fsys, repodir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, kerrors.WithMsg(err, "Failed to check repo")
	}
	cloned := err == nil
	if cloned && !info.IsDir() {
		return false, kerrors.WithKind(nil, repofetcher.ErrInvalidCache, fmt.Sprintf("Cached repo is not a directory: %s", repodir))
	}
	return cloned, nil
}

func (f *Fetcher) Fetch(ctx context.Context, spec repofetcher.RepoSpec) (fs.FS, error) {
	repospec, ok := spec.(RepoSpec)
	if !ok {
		return nil, kerrors.WithKind(nil, repofetcher.ErrInvalidRepoSpec, "Invalid spec type")
	}
	repodir, err := repospec.Key()
	if err != nil {
		return nil, err
	}
	cloned, err := f.checkRepoDir(repodir)
	if err != nil {
		return nil, err
	}
	repopath := path.Join(f.cacheDir, repodir)
	if !cloned || f.ForceFetch {
		if f.NoNetwork {
			if f.ForceFetch {
				return nil, kerrors.WithKind(nil, repofetcher.ErrNetworkRequired, "May not force fetch without network")
			}
			return nil, kerrors.WithKind(nil, repofetcher.ErrNetworkRequired, fmt.Sprintf("Cached repo not present: %s", repodir))
		}
		if cloned {
			if err := os.RemoveAll(filepath.FromSlash(repopath)); err != nil {
				return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed to clean existing dir: %s", repodir))
			}
		}
		if err := f.GitCmd.GitClone(ctx, repodir, repospec); err != nil {
			return nil, err
		}
	}
	rfsys, err := fs.Sub(f.fsys, repodir)
	if err != nil {
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed to get directory: %s", repodir))
	}
	return kfs.NewReadOnlyFS(kfs.NewMaskFS(kfs.New(rfsys, repopath), f.maskGitDir)), nil
}

func (f *Fetcher) maskGitDir(p string) (bool, error) {
	return p != f.gitDir && !strings.HasPrefix(p, f.gitDirPrefix), nil
}

type (
	GitBin struct {
		cacheDir string
		Bin      string
		Stdout   io.Writer
		Stderr   io.Writer
		Quiet    bool
	}
)

func NewGitBin(cacheDir string) *GitBin {
	return &GitBin{
		cacheDir: cacheDir,
		Bin:      "git",
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
		Quiet:    false,
	}
}

func (g *GitBin) GitClone(ctx context.Context, repodir string, repospec RepoSpec) error {
	args := make([]string, 0, 8)
	args = append(args, "clone", "--single-branch")
	if repospec.Commit != "" {
		args = append(args, "--branch", repospec.Branch, "--no-checkout")
		if repospec.ShallowSince != "" {
			args = append(args, "--shallow-since="+repospec.ShallowSince)
		}
	} else {
		args = append(args, "--branch", repospec.Tag, "--depth", "1")
	}
	args = append(args, repospec.Repo, repodir)
	if err := g.runCmd(
		exec.CommandContext(ctx, g.Bin, args...),
		filepath.FromSlash(g.cacheDir),
	); err != nil {
		return kerrors.WithMsg(err, fmt.Sprintf("Failed to clone repo: %s", repospec.Repo))
	}
	if repospec.Commit != "" {
		if err := g.runCmd(
			exec.CommandContext(ctx, g.Bin, "switch", "--detach", repospec.Commit),
			filepath.Join(filepath.FromSlash(g.cacheDir), repodir),
		); err != nil {
			return kerrors.WithMsg(err, fmt.Sprintf("Failed to checkout commit: %s", repospec.Commit))
		}
	}
	return nil
}

func (g *GitBin) runCmd(cmd *exec.Cmd, dir string) error {
	cmd.Dir = dir
	cmd.Env = os.Environ()
	if !g.Quiet {
		cmd.Stdout = g.Stdout
		cmd.Stderr = g.Stderr
	}
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}
