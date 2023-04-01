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

	"github.com/mitchellh/mapstructure"
	"xorkevin.dev/anvil/repofetcher"
	"xorkevin.dev/hunter2/h2streamhash"
	"xorkevin.dev/hunter2/h2streamhash/blake2bstream"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/kfs"
)

type (
	// Fetcher is a git repo fetcher
	Fetcher struct {
		fsys         fs.FS
		cacheDir     string
		verifier     *h2streamhash.Verifier
		gitDir       string
		gitDirPrefix string
		GitCmd       GitCmd
		NoNetwork    bool
		ForceFetch   bool
	}

	GitFetchOpts struct {
		Repo         string `mapstructure:"repo"`
		Tag          string `mapstructure:"tag"`
		Branch       string `mapstructure:"branch"`
		Commit       string `mapstructure:"commit"`
		ShallowSince string `mapstructure:"shallow_since"`
		Checksum     string `mapstructure:"checksum"`
	}

	GitCmd interface {
		GitClone(ctx context.Context, repodir string, opts GitFetchOpts) error
	}

	// Opt is a constructor option
	Opt = func(*Fetcher)
)

// New creates a new git [*Fetcher] which is rooted at a particular file system
func New(cacheDir string, opts ...Opt) *Fetcher {
	v := h2streamhash.NewVerifier()
	v.Register(blake2bstream.NewHasher(blake2bstream.Config{}))
	f := &Fetcher{
		fsys:         os.DirFS(cacheDir),
		cacheDir:     cacheDir,
		verifier:     v,
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

func (f *Fetcher) repoPath(opts GitFetchOpts) (string, error) {
	var s strings.Builder
	if opts.Repo == "" {
		return "", kerrors.WithKind(nil, repofetcher.ErrInvalidRepoSpec, "No repo specified")
	}
	s.WriteString(url.QueryEscape(opts.Repo))
	s.WriteString("@")
	if opts.Tag != "" {
		s.WriteString(url.QueryEscape(opts.Tag))
	} else if opts.Commit != "" {
		if opts.Branch == "" {
			return "", kerrors.WithKind(nil, repofetcher.ErrInvalidRepoSpec, "Branch missing for commit")
		}
		s.WriteString(url.QueryEscape(opts.Branch))
		s.WriteString("-")
		s.WriteString(url.QueryEscape(opts.Commit))
	} else {
		return "", kerrors.WithKind(nil, repofetcher.ErrInvalidRepoSpec, "No repo tag or commit specified")
	}
	return s.String(), nil
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

func (f *Fetcher) Fetch(ctx context.Context, opts map[string]any) (fs.FS, error) {
	var fetchOpts GitFetchOpts
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
				return nil, kerrors.WithKind(nil, repofetcher.ErrNetworkRequired, "May not force fetch without network")
			}
			return nil, kerrors.WithKind(nil, repofetcher.ErrNetworkRequired, fmt.Sprintf("Cached repo not present: %s", repodir))
		}
		if cloned {
			if err := os.RemoveAll(filepath.Join(f.cacheDir, repodir)); err != nil {
				return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed to clean existing dir: %s", repodir))
			}
		}
		if err := f.GitCmd.GitClone(ctx, repodir, fetchOpts); err != nil {
			return nil, err
		}
	}
	rfsys, err := fs.Sub(f.fsys, repodir)
	if err != nil {
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed to get directory: %s", repodir))
	}
	repopath := path.Join(f.cacheDir, repodir)
	rfsys = kfs.NewReadOnlyFS(kfs.NewMaskFS(kfs.New(rfsys, repopath), f.maskGitDir))
	if fetchOpts.Checksum != "" {
		if ok, err := repofetcher.MerkelTreeVerify(rfsys, f.verifier, fetchOpts.Checksum); err != nil {
			return nil, kerrors.WithMsg(err, "Failed computing repo checksum")
		} else if !ok {
			return nil, kerrors.WithMsg(nil, "Repo failed integrity check")
		}
	}
	return rfsys, nil
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

func (g *GitBin) GitClone(ctx context.Context, repodir string, opts GitFetchOpts) error {
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
	if err := g.runCmd(
		exec.CommandContext(ctx, g.Bin, args...),
		g.cacheDir,
	); err != nil {
		return kerrors.WithMsg(err, fmt.Sprintf("Failed to clone repo: %s", opts.Repo))
	}
	if opts.Commit != "" {
		if err := g.runCmd(
			exec.CommandContext(ctx, g.Bin, "switch", "--detach", opts.Commit),
			filepath.Join(g.cacheDir, repodir),
		); err != nil {
			return kerrors.WithMsg(err, fmt.Sprintf("Failed to checkout commit: %s", opts.Commit))
		}
	}
	return nil
}

func (g *GitBin) runCmd(cmd *exec.Cmd, dir string) error {
	if !g.Quiet {
		cmd.Stdout = g.Stdout
		cmd.Stderr = g.Stderr
	}
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}
