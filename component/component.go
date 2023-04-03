package component

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"path"

	"xorkevin.dev/anvil/repofetcher"
	"xorkevin.dev/anvil/util/kjson"
	"xorkevin.dev/kerrors"
)

const (
	repoKindLocalDir = "localdir"
	repoKindGit      = "git"

	configKindJsonnet = "jsonnet"
)

type (
	// configData is the shape of a generated config
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

func parseSubcomponent(ctx context.Context, cache *Cache, spec repofetcher.Spec, dir string, data ComponentData) ([]Component, error) {
	var compspec repofetcher.Spec
	var compname string
	if data.RepoKind == "" {
		if !fs.ValidPath(data.Path) {
			return nil, kerrors.WithKind(nil, ErrInvalidDir, fmt.Sprintf("Invalid repo dir %s for local subcomponent", data.Path))
		}
		compspec = spec
		compname = path.Join(dir, data.Path)
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
	c, err := ParseComponents(ctx, cache, compspec, compname, data.Args)
	if err != nil {
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed parsing subcomponent %s %s", compspec, compname))
	}
	return c, nil
}

// ParseComponents parses components
func ParseComponents(ctx context.Context, cache *Cache, spec repofetcher.Spec, name string, args map[string]any) ([]Component, error) {
	dir, name := path.Split(name)
	dir = path.Clean(dir)
	name = path.Clean(name)

	config, err := parseConfigFile(ctx, cache, spec, dir, name, args)
	if err != nil {
		return nil, err
	}

	// TODO: cycle detection on spec, dir, name

	var components []Component
	for _, i := range config.Components {
		c, err := parseSubcomponent(ctx, cache, spec, dir, i)
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

// Generate writes the generated templated files to a filesystem
func (c *Component) Generate(fsys WriteFS) error {
	data := configTplData{
		Vars: c.Vars,
	}
	for _, v := range c.Templates {
		if err := func() error {
			file, err := fsys.OpenFile(v.Output, generatedFileFlag, v.Mode)
			if err != nil {
				return fmt.Errorf("Invalid output file %s: %w", v.Output, err)
			}
			defer func() {
				if err := file.Close(); err != nil {
					log.Printf("Failed to close open file %s: %v", v.Output, err)
				}
			}()
			if err := v.Tpl.Execute(file, data); err != nil {
				return fmt.Errorf("Failed to generate template output %s: %w", v.Output, err)
			}
			return nil
		}(); err != nil {
			return err
		}
	}
	return nil
}
