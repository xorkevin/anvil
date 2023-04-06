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
		gitCmd       GitCmd
		noNetwork    bool
		forceFetch   bool
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
		GitClone(ctx context.Context, workingDir string, repodir string, repospec RepoSpec) error
	}

	// Opt is a constructor option
	Opt = func(*Fetcher)
)

// New creates a new git [*Fetcher] which is rooted at a particular file system
func New(fsys fs.FS, cacheDir string, opts ...Opt) *Fetcher {
	f := &Fetcher{
		fsys:         fsys,
		cacheDir:     cacheDir,
		gitDir:       ".git",
		gitDirPrefix: ".git/",
		gitCmd:       NewGitBin(),
		noNetwork:    false,
		forceFetch:   false,
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

func OptGitCmd(c GitCmd) Opt {
	return func(f *Fetcher) {
		f.gitCmd = c
	}
}

func OptNoNetwork(v bool) Opt {
	return func(f *Fetcher) {
		f.noNetwork = v
	}
}

func OptForceFetch(v bool) Opt {
	return func(f *Fetcher) {
		f.forceFetch = v
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
			return "", kerrors.WithKind(nil, repofetcher.ErrInvalidRepoSpec, fmt.Sprintf("Branch missing for commit for repo %s", o.Repo))
		}
		s.WriteString(url.QueryEscape(o.Branch))
		s.WriteString("-")
		s.WriteString(url.QueryEscape(o.Commit))
	} else {
		return "", kerrors.WithKind(nil, repofetcher.ErrInvalidRepoSpec, fmt.Sprintf("No repo tag or commit specified for repo %s", o.Repo))
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
	if !cloned || f.forceFetch {
		if f.noNetwork {
			if f.forceFetch {
				return nil, kerrors.WithKind(nil, repofetcher.ErrNetworkRequired, "May not force fetch without network")
			}
			return nil, kerrors.WithKind(nil, repofetcher.ErrNetworkRequired, fmt.Sprintf("Cached repo not present: %s", repodir))
		}
		if cloned {
			if err := kfs.RemoveAll(f.fsys, repodir); err != nil {
				return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed to clean existing dir: %s", repodir))
			}
		}
		if err := f.gitCmd.GitClone(ctx, f.cacheDir, repodir, repospec); err != nil {
			return nil, err
		}
	}
	rfsys, err := fs.Sub(f.fsys, repodir)
	if err != nil {
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed to get subdirectory: %s", repodir))
	}
	return kfs.NewReadOnlyFS(kfs.NewMaskFS(kfs.New(rfsys, path.Join(f.cacheDir, repodir)), f.maskGitDir)), nil
}

func (f *Fetcher) maskGitDir(p string) (bool, error) {
	return p != f.gitDir && !strings.HasPrefix(p, f.gitDirPrefix), nil
}

type (
	GitBin struct {
		bin    string
		quiet  bool
		Stdout io.Writer
		Stderr io.Writer
	}

	OptBin = func(b *GitBin)
)

func NewGitBin(opts ...OptBin) *GitBin {
	b := &GitBin{
		bin:    "git",
		quiet:  false,
		Stdout: os.Stderr, // send stdout to stderr for execed commands
		Stderr: os.Stderr,
	}
	for _, i := range opts {
		i(b)
	}
	return b
}

func OptBinName(name string) OptBin {
	return func(b *GitBin) {
		b.bin = name
	}
}

func OptBinQuiet(v bool) OptBin {
	return func(b *GitBin) {
		b.quiet = v
	}
}

func (g *GitBin) GitClone(ctx context.Context, workingDir string, repodir string, repospec RepoSpec) error {
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
		exec.CommandContext(ctx, g.bin, args...),
		filepath.FromSlash(workingDir),
	); err != nil {
		return kerrors.WithMsg(err, fmt.Sprintf("Failed to clone repo: %s", repospec.Repo))
	}
	if repospec.Commit != "" {
		if err := g.runCmd(
			exec.CommandContext(ctx, g.bin, "switch", "--detach", repospec.Commit),
			filepath.Join(filepath.FromSlash(workingDir), repodir),
		); err != nil {
			return kerrors.WithMsg(err, fmt.Sprintf("Failed to checkout commit %s for repo %s", repospec.Commit, repospec.Repo))
		}
	}
	return nil
}

func (g *GitBin) runCmd(cmd *exec.Cmd, dir string) error {
	cmd.Dir = dir
	cmd.Env = os.Environ()
	if !g.quiet {
		cmd.Stdout = g.Stdout
		cmd.Stderr = g.Stderr
	}
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}
