package component

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"xorkevin.dev/anvil/confengine"
	"xorkevin.dev/anvil/confengine/jsonnetengine"
	"xorkevin.dev/anvil/repofetcher"
	"xorkevin.dev/anvil/repofetcher/gitfetcher"
	"xorkevin.dev/anvil/repofetcher/localdir"
	"xorkevin.dev/anvil/util/kjson"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/kfs"
	"xorkevin.dev/klog"
)

// ErrImportCycle is returned when component dependencies form a cycle
var ErrImportCycle errImportCycle

type (
	errImportCycle struct{}
)

func (e errImportCycle) Error() string {
	return "Import cycle"
}

const (
	repoKindLocalDir  = "localdir"
	configKindJsonnet = "jsonnet"
)

type (
	// configData is the shape of a generated config
	configData struct {
		Version    string          `json:"version"`
		Templates  []Template      `json:"templates"`
		Components []componentData `json:"components"`
	}

	// componentData is the shape of a generated config component
	componentData struct {
		Kind string          `json:"kind"`
		Repo json.RawMessage `json:"repo"`
		Path string          `json:"path"`
		Args map[string]any  `json:"args"`
	}

	// Component is a package of files to generate
	Component struct {
		Spec      repofetcher.Spec
		Dir       string
		Templates []Template
	}

	// Template is a file to generate
	Template struct {
		Kind   string         `json:"kind"`
		Path   string         `json:"path"`
		Args   map[string]any `json:"args"`
		Output string         `json:"output"`
	}
)

func parseConfigFile(ctx context.Context, cache *Cache, spec repofetcher.Spec, dir string, name string, args map[string]any) (*configData, error) {
	eng, err := cache.Get(ctx, configKindJsonnet, spec, dir)
	if err != nil {
		return nil, err
	}
	outbytes, err := eng.Exec(ctx, name, args)
	if err != nil {
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed executing component config %s %s/%s", spec, dir, name))
	}
	config := &configData{}
	if err := kjson.Unmarshal(outbytes, config); err != nil {
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Invalid output for component config %s %s/%s", spec, dir, name))
	}
	return config, nil
}

func parseSubcomponent(ctx context.Context, cache *Cache, ss *stackSet, spec repofetcher.Spec, dir string, data componentData) ([]Component, error) {
	var compspec repofetcher.Spec
	var compname string
	if data.Kind == repoKindLocalDir {
		return nil, kerrors.WithKind(nil, repofetcher.ErrUnknownKind, fmt.Sprintf("Invalid repo kind: %s", data.Kind))
	} else if data.Kind == "" {
		compspec = spec
		compname = path.Join(dir, data.Path)
		if !fs.ValidPath(compname) {
			return nil, kerrors.WithKind(nil, ErrInvalidDir, fmt.Sprintf("Invalid repo dir %s for local subcomponent", data.Path))
		}
	} else {
		var err error
		compspec, err = cache.Parse(data.Kind, data.Repo)
		if err != nil {
			return nil, kerrors.WithMsg(err, fmt.Sprintf("Invalid %s subcomponent", data.Kind))
		}
		if !fs.ValidPath(data.Path) {
			return nil, kerrors.WithKind(nil, ErrInvalidDir, fmt.Sprintf("Invalid repo dir %s for subcomponent %s", data.Path, compspec))
		}
		compname = data.Path
	}
	c, err := parseComponentsRec(ctx, cache, ss, compspec, compname, data.Args)
	if err != nil {
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed parsing subcomponent %s %s", compspec, compname))
	}
	return c, nil
}

func componentKey(spec repofetcher.Spec, dir string, name string) string {
	var s strings.Builder
	s.WriteString(spec.String())
	s.WriteString(":")
	s.WriteString(path.Join(dir, name))
	return s.String()
}

func parseComponentsRec(ctx context.Context, cache *Cache, ss *stackSet, spec repofetcher.Spec, name string, args map[string]any) (_ []Component, retErr error) {
	dir, name := path.Split(name)
	dir = path.Clean(dir)
	name = path.Clean(name)

	config, err := parseConfigFile(ctx, cache, spec, dir, name, args)
	if err != nil {
		return nil, err
	}

	compkey := componentKey(spec, dir, name)
	if !ss.Push(compkey) {
		return nil, kerrors.WithKind(nil, ErrImportCycle, fmt.Sprintf("Import cycle on repo %s %s/%s", spec, dir, name))
	}
	defer func() {
		v, ok := ss.Pop()
		if !ok {
			retErr = errors.Join(retErr, kerrors.WithKind(nil, ErrImportCycle, fmt.Sprintf("Failed checking import cycle due to missing element on repo %s %s/%s", spec, dir, name)))
		} else if v != compkey {
			retErr = errors.Join(retErr, kerrors.WithKind(nil, ErrImportCycle, fmt.Sprintf("Failed checking import cycle due to mismatched element on repo %s %s/%s", spec, dir, name)))
		}
	}()

	var components []Component
	for _, i := range config.Components {
		c, err := parseSubcomponent(ctx, cache, ss, spec, dir, i)
		if err != nil {
			return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed parsing subcomponent of %s %s/%s", spec, dir, name))
		}
		components = append(components, c...)
	}

	components = append(components, Component{
		Spec:      spec,
		Dir:       dir,
		Templates: config.Templates,
	})
	return components, nil
}

// ParseComponents parses component configs to [Component]
func ParseComponents(ctx context.Context, cache *Cache, spec repofetcher.Spec, name string) ([]Component, error) {
	return parseComponentsRec(ctx, cache, newStackSet(), spec, name, nil)
}

func writeComponent(ctx context.Context, log *klog.LevelLogger, cache *Cache, fsys fs.FS, component Component, dryrun bool) error {
	ctx = klog.CtxWithAttrs(ctx, klog.AString("repo", component.Spec.String()), klog.AString("dir", component.Dir))
	log.Info(ctx, "Writing component")
	for _, i := range component.Templates {
		eng, err := cache.Get(ctx, i.Kind, component.Spec, component.Dir)
		if err != nil {
			return err
		}
		outbytes, err := eng.Exec(ctx, i.Path, i.Args)
		if err != nil {
			return kerrors.WithMsg(err, fmt.Sprintf("Failed executing component template %s %s/%s", component.Spec, component.Dir, i.Path))
		}
		if dryrun {
			log.Info(ctx, "Dry run write template", klog.AString("path", i.Path), klog.AString("output", i.Output))
		} else {
			if err := kfs.WriteFile(fsys, i.Output, outbytes, 0o644); err != nil {
				return kerrors.WithMsg(err, fmt.Sprintf("Failed writing component template output for %s %s/%s to %s", component.Spec, component.Dir, i.Path, i.Output))
			}
			log.Info(ctx, "Wrote template", klog.AString("path", i.Path), klog.AString("output", i.Output))
		}
	}
	return nil
}

// WriteComponents writes components to an fs
func WriteComponents(ctx context.Context, log klog.Logger, cache *Cache, fsys fs.FS, components []Component, dryrun bool) error {
	l := klog.NewLevelLogger(log)
	for _, i := range components {
		if err := writeComponent(ctx, l, cache, fsys, i, dryrun); err != nil {
			return err
		}
	}
	return nil
}

type (
	// Opts holds generation opts
	Opts struct {
		DryRun           bool
		NoNetwork        bool
		ForceFetch       bool
		RepoChecksumFile string
		GitDir           string
		GitBin           string
		GitBinQuiet      bool
		JsonnetLibName   string
	}

	// RepoChecksumData is the shape of a repo checksum file
	RepoChecksumData struct {
		Repos []repofetcher.RepoChecksum `json:"repos"`
	}
)

func parseRepoChecksumFile(name string) (map[string]string, error) {
	b, err := os.ReadFile(filepath.FromSlash(name))
	if err != nil {
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed to read repo checksum file: %s", name))
	}
	var data RepoChecksumData
	if err := kjson.Unmarshal(b, &data); err != nil {
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Malformed repo checksum file: %s", name))
	}
	res := map[string]string{}
	for _, i := range data.Repos {
		res[i.Key] = i.Sum
	}
	return res, nil
}

func writeRepoChecksumFile(name string, repos []repofetcher.RepoChecksum) error {
	b, err := kjson.Marshal(RepoChecksumData{
		Repos: repos,
	})
	if err != nil {
		return kerrors.WithMsg(err, "Failed to construct repo checksum data")
	}
	if err := os.WriteFile(filepath.FromSlash(name), b, 0o644); err != nil {
		return kerrors.WithMsg(err, fmt.Sprintf("Failed to write repo checksum file: %s", name))
	}
	return nil
}

// Generate reads configs and writes components to the filesystem
func Generate(ctx context.Context, log klog.Logger, output, input, cachedir string, opts Opts) error {
	l := klog.NewLevelLogger(log)

	var checksums map[string]string
	if opts.RepoChecksumFile != "" {
		var err error
		checksums, err = parseRepoChecksumFile(opts.RepoChecksumFile)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			// file does not exist
			checksums = nil
			l.Info(ctx, "Repo checksum file not found", klog.AString("file", opts.RepoChecksumFile))
		} else {
			l.Info(ctx, "Using existing repo checksum file", klog.AString("file", opts.RepoChecksumFile))
		}
	}

	local, name := path.Split(input)
	gitdir := path.Join(cachedir, "repos", "git")

	cache := NewCache(
		repofetcher.NewCache(
			repofetcher.Map{
				repoKindLocalDir: localdir.New(kfs.NewReadOnlyFS(kfs.DirFS(local))),
				"git": gitfetcher.New(
					kfs.NewReadOnlyFS(kfs.DirFS(gitdir)),
					log.Sublogger("gitfetcher"),
					gitfetcher.OptGitDir(opts.GitDir),
					gitfetcher.OptGitCmd(gitfetcher.NewGitBin(
						gitdir,
						gitfetcher.OptBinName(opts.GitBin),
						gitfetcher.OptBinQuiet(opts.GitBinQuiet),
					)),
					gitfetcher.OptNoNetwork(opts.NoNetwork),
					gitfetcher.OptForceFetch(opts.ForceFetch),
				),
			},
			map[string]struct{}{
				repoKindLocalDir: {},
			},
			checksums,
		),
		confengine.Map{
			configKindJsonnet: jsonnetengine.Builder{jsonnetengine.OptLibName(opts.JsonnetLibName)},
			"jsonnetstr":      jsonnetengine.Builder{jsonnetengine.OptLibName(opts.JsonnetLibName), jsonnetengine.OptStrOut(true)},
		},
	)

	components, err := ParseComponents(
		ctx,
		cache,
		repofetcher.Spec{Kind: repoKindLocalDir, RepoSpec: localdir.RepoSpec{}},
		name,
	)
	if err != nil {
		return err
	}

	if opts.RepoChecksumFile != "" {
		if opts.DryRun {
			l.Info(ctx, "Dry run write repo sum file", klog.AString("file", opts.RepoChecksumFile))
		} else {
			if err := writeRepoChecksumFile(opts.RepoChecksumFile, cache.repos.Sums()); err != nil {
				return kerrors.WithMsg(err, fmt.Sprintf("Failed writing repo sum file: %s", opts.RepoChecksumFile))
			}
			l.Info(ctx, "Wrote repo sum file", klog.AString("file", opts.RepoChecksumFile))
		}
	}

	if err := WriteComponents(ctx, log, cache, kfs.DirFS(output), components, opts.DryRun); err != nil {
		return err
	}
	return nil
}
