package component

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"text/template"

	"gopkg.in/yaml.v3"
)

const (
	fileExtJson = ".json"
	fileExtYaml = ".yaml"
	fileExtYml  = ".yml"
)

const (
	componentKindLocal = "local"
	componentKindGit   = "git"
)

var (
	// ErrInvalidExt is returned when attempting to parse a file with an invalid extension
	ErrInvalidExt = errors.New("Invalid config extension")
)

type (
	// ConfigData is the shape of the component config
	ConfigData struct {
		Version   string                 `json:"version" yaml:"version"`
		Vars      map[string]interface{} `json:"vars" yaml:"vars"`
		ConfigTpl string                 `json:"configtpl" yaml:"configtpl"`
	}

	// ConfigFile represents a parsed component config
	ConfigFile struct {
		Name       string
		ConfigData ConfigData
		Dir        fs.FS
		ConfigTpl  *template.Template
	}

	// ConfigTplData is the input data of a config template
	ConfigTplData struct {
		Vars map[string]interface{}
	}

	// TemplateData is the shape of a generated config template
	TemplateData struct {
		Path   string `json:"path" yaml:"path"`
		Output string `json:"output" yaml:"output"`
	}

	// ComponentData is the shape of a generated config component
	ComponentData struct {
		Kind string                 `json:"kind" yaml:"kind"`
		Path string                 `json:"path" yaml:"path"`
		Vars map[string]interface{} `json:"vars" yaml:"vars"`
	}

	// GenConfigData is the shape of a generated config
	GenConfigData struct {
		Templates  map[string]TemplateData  `json:"templates" yaml:"templates"`
		Components map[string]ComponentData `json:"components" yaml:"components"`
	}

	// Component is a parsed component
	Component struct {
		Kind       string
		Path       string
		Vars       map[string]interface{}
		Templates  map[string]TemplateData
		Components map[string]Patch
	}

	// Patch is the shape of a patch file
	Patch struct {
		Vars       map[string]interface{}  `json:"vars" yaml:"vars"`
		Templates  map[string]TemplateData `json:"templates" yaml:"templates"`
		Components map[string]Patch        `json:"components" yaml:"components"`
	}

	// WriteFS is a file system that may be read from and written to
	WriteFS interface {
		OpenFile(name string, flag int, perm fs.FileMode) (io.WriteCloser, error)
	}
)

func decodeJSONorYAML(r io.Reader, ext string, target interface{}) error {
	switch ext {
	case fileExtJson:
		if err := json.NewDecoder(r).Decode(target); err != nil {
			return fmt.Errorf("Invalid JSON: %w", err)
		}
	case fileExtYaml, fileExtYml:
		if err := yaml.NewDecoder(r).Decode(target); err != nil {
			return fmt.Errorf("Invalid YAML: %w", err)
		}
	default:
		return fmt.Errorf("%w: %s", ErrInvalidExt, ext)
	}
	return nil
}

func decodeJSONorYAMLFile(fsys fs.FS, path string, target interface{}) error {
	file, err := fsys.Open(path)
	if err != nil {
		return fmt.Errorf("Invalid file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Failed to close open file %s: %v", path, err)
		}
	}()
	return decodeJSONorYAML(file, filepath.Ext(path), target)
}

func ParseConfigFile(fsys fs.FS, path string) (*ConfigFile, error) {
	dirpath := filepath.Dir(path)
	dir, err := fs.Sub(fsys, dirpath)
	if err != nil {
		return nil, fmt.Errorf("Failed to open dir %s: %w", dirpath, err)
	}

	var config ConfigData
	if err := decodeJSONorYAMLFile(fsys, path, &config); err != nil {
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
		Name:       dirpath,
		ConfigData: config,
		Dir:        dir,
		ConfigTpl:  cfgtpl,
	}, nil
}

func ParsePatchFile(fsys fs.FS, path string) (*Patch, error) {
	var patch Patch
	if err := decodeJSONorYAMLFile(fsys, path, &patch); err != nil {
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
		if v.Output != "" {
			t.Output = v.Output
		}
		merged[k] = t
	}
	return merged
}

func mergeComponents(components map[string]ComponentData, patch map[string]Patch) map[string]Component {
	merged := map[string]Component{}
	for k, v := range components {
		merged[k] = Component{
			Kind: v.Kind,
			Path: v.Path,
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

func (c *ConfigFile) Generate(fsys WriteFS, patch *Patch) (map[string]Component, error) {
	if patch == nil {
		patch = &Patch{}
	}

	vars := jsonMergePatchObj(c.ConfigData.Vars, patch.Vars)

	data := ConfigTplData{
		Vars: vars,
	}

	var gencfg GenConfigData
	if c.ConfigTpl != nil {
		b := &bytes.Buffer{}
		if err := c.ConfigTpl.Execute(b, data); err != nil {
			return nil, fmt.Errorf("Failed to generate config: %w", err)
		}
		if err := decodeJSONorYAML(b, filepath.Ext(c.ConfigData.ConfigTpl), &gencfg); err != nil {
			return nil, fmt.Errorf("Invalid generated config %s: %w", c.ConfigData.ConfigTpl, err)
		}
	}

	for _, v := range mergeTemplates(gencfg.Templates, patch.Templates) {
		t, err := template.New(v.Path).ParseFS(c.Dir, v.Path)
		if err != nil {
			return nil, fmt.Errorf("Invalid template %s: %w", v.Path, err)
		}
		if err := func() error {
			file, err := fsys.OpenFile(v.Output, os.O_RDWR|os.O_CREATE, 0644)
			if err != nil {
				return fmt.Errorf("Invalid file: %w", err)
			}
			defer func() {
				if err := file.Close(); err != nil {
					log.Printf("Failed to close open file %s: %v", v.Output, err)
				}
			}()
			b := bufio.NewWriter(file)
			if err := t.Execute(b, data); err != nil {
				return fmt.Errorf("Failed to generate template output: %w", err)
			}
			if err := b.Flush(); err != nil {
				return fmt.Errorf("Failed to write to file %s: %w", v.Output, err)
			}
			return nil
		}(); err != nil {
			return nil, err
		}
	}

	return mergeComponents(gencfg.Components, patch.Components), nil
}
