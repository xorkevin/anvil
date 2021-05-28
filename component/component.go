package component

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"text/template"

	"xorkevin.dev/anvil/configfile"
)

const (
	componentKindLocal = "local"
	componentKindGit   = "git"
)

const (
	generatedFileMode = 0644
	generatedFileFlag = os.O_RDWR | os.O_CREATE
)

type (
	// templateCache caches parsed templates by path
	templateCache struct {
		dir   fs.FS
		cache map[string]*template.Template
	}

	// configData is the shape of the component config
	configData struct {
		Version   string                 `json:"version" yaml:"version"`
		Vars      map[string]interface{} `json:"vars" yaml:"vars"`
		ConfigTpl string                 `json:"configtpl" yaml:"configtpl"`
	}

	// ConfigFile represents a parsed component config
	ConfigFile struct {
		Version   string
		Name      string
		Vars      map[string]interface{}
		path      string
		configTpl *template.Template
		tplcache  *templateCache
	}

	// configTplData is the input data of a config template
	configTplData struct {
		Vars map[string]interface{}
	}

	// TemplateData is the shape of a generated config template
	TemplateData struct {
		Path   string `json:"path" yaml:"path"`
		Output string `json:"output" yaml:"output"`
	}

	// componentData is the shape of a generated config component
	componentData struct {
		Kind string                 `json:"kind" yaml:"kind"`
		Repo string                 `json:"repo,omitempty" yaml:"repo,omitempty"`
		Ref  string                 `json:"ref,omitempty" yaml:"ref,omitempty"`
		Path string                 `json:"path" yaml:"path"`
		Vars map[string]interface{} `json:"vars" yaml:"vars"`
	}

	// genConfigData is the shape of a generated config
	genConfigData struct {
		Templates  map[string]TemplateData  `json:"templates" yaml:"templates"`
		Components map[string]componentData `json:"components" yaml:"components"`
	}

	// Template is a parsed template file
	Template struct {
		Tpl    *template.Template
		Output string
	}

	// RepoPath represents a repo component path
	RepoPath struct {
		Kind string
		Repo string
		Ref  string
		Path string
	}

	// Subcomponent is a parsed sub component
	Subcomponent struct {
		Src        RepoPath
		Vars       map[string]interface{}
		Templates  map[string]TemplateData
		Components map[string]Patch
	}

	// Component is an instantiated component
	Component struct {
		Vars      map[string]interface{}
		Templates map[string]Template
	}

	// Patch is the shape of a patch file
	Patch struct {
		Vars       map[string]interface{}  `json:"vars" yaml:"vars"`
		Templates  map[string]TemplateData `json:"templates" yaml:"templates"`
		Components map[string]Patch        `json:"components" yaml:"components"`
	}
)

func (r RepoPath) String() string {
	return fmt.Sprintf("[%s] %s (%s) %s", r.Kind, r.Repo, r.Ref, r.Path)
}

func newTemplateCache(dir fs.FS) *templateCache {
	return &templateCache{
		dir:   dir,
		cache: map[string]*template.Template{},
	}
}

func (c *templateCache) Parse(path string) (*template.Template, error) {
	if t, ok := c.cache[path]; ok {
		return t, nil
	}
	t, err := template.New(filepath.Base(path)).ParseFS(c.dir, path)
	if err != nil {
		return nil, fmt.Errorf("Invalid template %s: %w", path, err)
	}
	c.cache[path] = t
	return t, nil
}

// ParseConfigFile parses a config file in a filesystem
func ParseConfigFile(fsys fs.FS, path string) (*ConfigFile, error) {
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
			return nil, fmt.Errorf("Invalid config template %s: %w", config.ConfigTpl, err)
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

// ParsePatchFile parses a patch file in a filesystem
func ParsePatchFile(fsys fs.FS, path string) (*Patch, error) {
	var patch Patch
	if err := configfile.DecodeJSONorYAMLFile(fsys, path, &patch); err != nil {
		return nil, fmt.Errorf("Invalid patch file %s: %w", path, err)
	}
	return &patch, nil
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
		t.Vars = jsonMergePatchObj(t.Vars, v.Vars)
		t.Templates = v.Templates
		t.Components = v.Components
	}
	return merged
}

// Init initializes a component instance with variables
func (c *ConfigFile) Init(patch *Patch) (*Component, []Subcomponent, error) {
	if patch == nil {
		patch = &Patch{}
	}

	vars := jsonMergePatchObj(c.Vars, patch.Vars)

	var gencfg genConfigData
	if c.configTpl != nil {
		b := &bytes.Buffer{}
		data := configTplData{
			Vars: vars,
		}
		if err := c.configTpl.Execute(b, data); err != nil {
			return nil, nil, fmt.Errorf("Failed to generate config: %w", err)
		}
		if err := configfile.DecodeJSONorYAML(b, filepath.Ext(c.path), &gencfg); err != nil {
			return nil, nil, fmt.Errorf("Invalid generated config %s: %w", c.path, err)
		}
	}

	tpls := map[string]Template{}
	for k, v := range mergeTemplates(gencfg.Templates, patch.Templates) {
		t, err := c.tplcache.Parse(v.Path)
		if err != nil {
			return nil, nil, err
		}
		tpls[k] = Template{
			Tpl:    t,
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
			file, err := fsys.OpenFile(v.Output, generatedFileFlag, generatedFileMode)
			if err != nil {
				return fmt.Errorf("Invalid file %s: %w", v.Output, err)
			}
			defer func() {
				if err := file.Close(); err != nil {
					log.Printf("Failed to close open file %s: %v", v.Output, err)
				}
			}()
			b := bufio.NewWriter(file)
			if err := v.Tpl.Execute(b, data); err != nil {
				return fmt.Errorf("Failed to generate template output: %w", err)
			}
			if err := b.Flush(); err != nil {
				return fmt.Errorf("Failed to write to file %s: %w", v.Output, err)
			}
			return nil
		}(); err != nil {
			return err
		}
	}
	return nil
}
