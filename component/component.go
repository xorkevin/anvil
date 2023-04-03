package component

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"path/filepath"
	"sort"
	"text/template"

	"xorkevin.dev/anvil/util/kjson"
)

const (
	repoKindLocal    = "local"
	repoKindLocalDir = "localdir"
	repoKindGit      = "git"
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
		Kind   string `json:"kind"`
		Path   string `json:"path"`
		Output string `json:"output"`
	}

	// ComponentData is the shape of a generated config component
	ComponentData struct {
		Kind     string          `json:"kind"`
		RepoKind string          `json:"repokind"`
		Repo     json.RawMessage `json:"repo"`
		Dir      string          `json:"dir"`
		Path     string          `json:"path"`
		Args     map[string]any  `json:"args"`
	}
)

// ParseConfigFile parses a config file in a filesystem
func ParseConfigFile(fsys fs.FS, path string) (*ConfigData, error) {
	dirpath := filepath.Dir(path)
	dir, err := fs.Sub(fsys, dirpath)
	if err != nil {
		return nil, fmt.Errorf("Failed to open dir %s: %w", dirpath, err)
	}

	var config configData
	if err := configfile.DecodeJSONorYAMLFile(fsys, path, &config); err != nil {
		return nil, fmt.Errorf("Invalid config %s: %w", path, err)
	}

	var cfgtpl *template.Template
	if config.ConfigTpl != "" {
		var err error
		cfgtpl, err = template.New(config.ConfigTpl).ParseFS(dir, config.ConfigTpl)
		if err != nil {
			return nil, fmt.Errorf("Invalid config template %s %s: %w", dirpath, config.ConfigTpl, err)
		}
	}

	return &ConfigFile{
		Version:   config.Version,
		Name:      dirpath,
		Vars:      config.Vars,
		path:      config.ConfigTpl,
		configTpl: cfgtpl,
		tplcache:  newTemplateCache(dir),
	}, nil
}

func mergeTemplates(tpls, patch map[string]TemplateData) map[string]TemplateData {
	merged := map[string]TemplateData{}
	for k, v := range tpls {
		merged[k] = v
	}
	for k, v := range patch {
		t := merged[k]
		if v.Path != "" {
			t.Path = v.Path
		}
		if v.Output != "" {
			t.Output = v.Output
		}
		merged[k] = t
	}
	return merged
}

func mergeSubcomponents(components map[string]componentData, patch map[string]Patch) map[string]Subcomponent {
	merged := map[string]Subcomponent{}
	for k, v := range components {
		merged[k] = Subcomponent{
			Src: RepoPath{
				Kind: v.Kind,
				Repo: v.Repo,
				Ref:  v.Ref,
				Path: v.Path,
			},
			Vars: v.Vars,
		}
	}
	for k, v := range patch {
		t := merged[k]
		t.Vars = kjson.MergePatchObj(t.Vars, v.Vars)
		t.Templates = v.Templates
		t.Components = v.Components
	}
	return merged
}

// Init initializes a component instance with variables
func (c *ConfigFile) Init() (*Component, []Subcomponent, error) {
	vars := kjson.MergePatchObj(c.Vars, patch.Vars)

	var gencfg genConfigData
	if c.configTpl != nil {
		b := &bytes.Buffer{}
		data := configTplData{
			Vars: vars,
		}
		if err := c.configTpl.Execute(b, data); err != nil {
			return nil, nil, fmt.Errorf("Failed to generate config %s %s: %w", c.Name, c.path, err)
		}
		if err := configfile.DecodeJSONorYAML(b, filepath.Ext(c.path), &gencfg); err != nil {
			return nil, nil, fmt.Errorf("Invalid generated config %s %s: %w", c.Name, c.path, err)
		}
	}

	tpls := map[string]Template{}
	for k, v := range mergeTemplates(gencfg.Templates, patch.Templates) {
		t, err := c.tplcache.Parse(v.Path)
		if err != nil {
			return nil, nil, err
		}
		tpls[k] = Template{
			Tpl:    t.tpl,
			Mode:   t.mode,
			Output: v.Output,
		}
	}

	components := mergeSubcomponents(gencfg.Components, patch.Components)
	keys := make([]string, 0, len(components))
	for k := range components {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	deps := make([]Subcomponent, 0, len(components))
	for _, i := range keys {
		deps = append(deps, components[i])
	}

	return &Component{
		Vars:      vars,
		Templates: tpls,
	}, deps, nil
}

// Patch returns the subcomponent patch
func (s *Subcomponent) Patch() *Patch {
	return &Patch{
		Vars:       s.Vars,
		Templates:  s.Templates,
		Components: s.Components,
	}
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
