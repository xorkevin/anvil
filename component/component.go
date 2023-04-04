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
	gitfetcher "xorkevin.dev/anvil/repofetcher/gitfecher"
	"xorkevin.dev/anvil/repofetcher/localdir"
	"xorkevin.dev/anvil/util/kjson"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/kfs"
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
	configKindJsonnet = "jsonnet"
)

type (
	// ConfigData is the shape of a generated config
	ConfigData struct {
		Version    string          `json:"version"`
		Templates  []TemplateData  `json:"templates"`
		Components []ComponentData `json:"components"`
	}

	// TemplateData is the shape of a generated config template
	TemplateData struct {
		Kind   string         `json:"kind"`
		Path   string         `json:"path"`
		Args   map[string]any `json:"args"`
		Output string         `json:"output"`
	}

	// ComponentData is the shape of a generated config component
	ComponentData struct {
		Kind     string          `json:"kind"`
		RepoKind string          `json:"repokind"`
		Repo     json.RawMessage `json:"repo"`
		Path     string          `json:"path"`
		Args     map[string]any  `json:"args"`
	}

	// Component is a package of files to generate
	Component struct {
		Spec      repofetcher.Spec
		Dir       string
		Templates []TemplateData
	}
)

func parseConfigFile(ctx context.Context, cache *Cache, spec repofetcher.Spec, dir string, name string, args map[string]any) (*ConfigData, error) {
	eng, err := cache.Get(ctx, configKindJsonnet, spec, dir)
	if err != nil {
		return nil, err
	}
	outbytes, err := eng.Exec(name, args)
	if err != nil {
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed executing component config %s %s/%s", spec, dir, name))
	}
	config := &ConfigData{}
	if err := kjson.Unmarshal(outbytes, config); err != nil {
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Invalid output for component config %s %s/%s", spec, dir, name))
	}
	return config, nil
}

func parseSubcomponent(ctx context.Context, cache *Cache, ss *stackSet, spec repofetcher.Spec, dir string, data ComponentData) ([]Component, error) {
	var compspec repofetcher.Spec
	var compname string
	if data.RepoKind == "" {
		compspec = spec
		compname = path.Join(dir, data.Path)
		if !fs.ValidPath(compname) {
			return nil, kerrors.WithKind(nil, ErrInvalidDir, fmt.Sprintf("Invalid repo dir %s for local subcomponent", data.Path))
		}
	} else {
		var err error
		compspec, err = cache.Parse(data.RepoKind, data.Repo)
		if err != nil {
			return nil, kerrors.WithMsg(err, fmt.Sprintf("Invalid %s subcomponent", data.RepoKind))
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
func ParseComponents(ctx context.Context, cache *Cache, spec repofetcher.Spec, name string, args map[string]any) ([]Component, error) {
	return parseComponentsRec(ctx, cache, newStackSet(), spec, name, args)
}

func writeComponent(ctx context.Context, cache *Cache, fsys fs.FS, component Component) error {
	for _, i := range component.Templates {
		eng, err := cache.Get(ctx, i.Kind, component.Spec, component.Dir)
		if err != nil {
			return err
		}
		outbytes, err := eng.Exec(i.Path, i.Args)
		if err != nil {
			return kerrors.WithMsg(err, fmt.Sprintf("Failed executing component template %s %s/%s", component.Spec, component.Dir, i.Path))
		}
		if err := kfs.WriteFile(fsys, i.Output, outbytes, 0o644); err != nil {
			return kerrors.WithMsg(err, fmt.Sprintf("Failed writing component template output for %s %s/%s to %s", component.Spec, component.Dir, i.Path, i.Output))
		}
	}
	return nil
}

// WriteComponents writes components to an fs
func WriteComponents(ctx context.Context, cache *Cache, fsys fs.FS, components []Component) error {
	for _, i := range components {
		if err := writeComponent(ctx, cache, fsys, i); err != nil {
			return err
		}
	}
	return nil
}

type (
	// Opts holds generation opts
	Opts struct {
		GitDir           string
		GitBin           string
		GitBinQuiet      bool
		NoNetwork        bool
		ForceFetch       bool
		RepoChecksumFile string
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

// Generate reads configs and writes components to the filesystem
func Generate(ctx context.Context, output, local, cachedir, name string, opts Opts) error {
	var checksums map[string]string
	if opts.RepoChecksumFile != "" {
		var err error
		checksums, err = parseRepoChecksumFile(opts.RepoChecksumFile)
		if err != nil {
			return err
		}
	}

	outputfs := kfs.New(os.DirFS(filepath.FromSlash(output)), output)

	cache := NewCache(
		repofetcher.NewCache(
			repofetcher.Map{
				"localdir": localdir.New(local),
				"git": gitfetcher.New(
					path.Join(cachedir, "repos", "git"),
					gitfetcher.OptGitDir(opts.GitDir),
					gitfetcher.OptGitCmd(gitfetcher.NewGitBin(
						gitfetcher.OptBinName(opts.GitBin),
						gitfetcher.OptBinQuiet(opts.GitBinQuiet),
					)),
					gitfetcher.OptNoNetwork(opts.NoNetwork),
					gitfetcher.OptForceFetch(opts.ForceFetch),
				),
			},
			map[string]struct{}{
				"localdir": {},
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
		repofetcher.Spec{Kind: "localdir", RepoSpec: localdir.RepoSpec{}},
		name,
		nil,
	)
	if err != nil {
		return err
	}
	if err := WriteComponents(ctx, cache, outputfs, components); err != nil {
		return err
	}
	return nil
}
