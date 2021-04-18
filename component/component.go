package component

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
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
	ErrInvalidExt = errors.New("Invalid config extension")
	ErrVarsPatch  = errors.New("Invalid vars patch")
)

type (
	Template struct {
		Path   string `json:"path" yaml:"path"`
		Output string `json:"output" yaml:"output"`
	}

	Component struct {
		Kind string                 `json:"kind" yaml:"kind"`
		Path string                 `json:"path" yaml:"path"`
		Vars map[string]interface{} `json:"vars" yaml:"vars"`
	}

	ConfigData struct {
		Version    string                 `json:"version" yaml:"version"`
		Vars       map[string]interface{} `json:"vars" yaml:"vars"`
		Templates  map[string]Template    `json:"templates" yaml:"templates"`
		Components string                 `json:"components" yaml:"components"`
	}

	ComponentData struct {
		Components map[string]Component `json:"components" yaml:"components"`
	}

	Config struct {
		Name       string
		ConfigData ConfigData
		Dir        fs.FS
		Components *template.Template
	}

	Patch struct {
		Vars       map[string]interface{} `json:"vars" yaml:"vars"`
		Templates  map[string]Template    `json:"templates" yaml:"templates"`
		Components map[string]Patch       `json:"components" yaml:"components"`
	}
)

func decodeJSONorYAML(fsys fs.FS, path string, target interface{}) error {
	file, err := fsys.Open(path)
	if err != nil {
		return fmt.Errorf("Invalid file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Failed to close open file %s: %v", path, err)
		}
	}()

	switch ext := filepath.Ext(path); ext {
	case fileExtJson:
		if err := json.NewDecoder(file).Decode(target); err != nil {
			return fmt.Errorf("Invalid JSON: %w", err)
		}
	case fileExtYaml, fileExtYml:
		if err := yaml.NewDecoder(file).Decode(target); err != nil {
			return fmt.Errorf("Invalid YAML: %w", err)
		}
	default:
		return fmt.Errorf("%w: %s", ErrInvalidExt, ext)
	}

	return nil
}

func ParseConfigFile(fsys fs.FS, path string) (*Config, error) {
	dirpath := filepath.Dir(path)
	dir, err := fs.Sub(fsys, dirpath)
	if err != nil {
		return nil, fmt.Errorf("Failed to open dir %s: %w", dirpath, err)
	}

	var config ConfigData
	if err := decodeJSONorYAML(fsys, path, &config); err != nil {
		return nil, fmt.Errorf("Invalid config %s: %w", path, err)
	}

	var components *template.Template
	if config.Components != "" {
		var err error
		components, err = template.New("anvilcomponents").ParseFS(dir, config.Components)
		if err != nil {
			return nil, fmt.Errorf("Invalid components config %s: %w", config.Components, err)
		}
	}

	return &Config{
		Name:       dirpath,
		ConfigData: config,
		Dir:        dir,
		Components: components,
	}, nil
}

func ParsePatchFile(rootdir fs.FS, path string) (*Patch, error) {
	file, err := rootdir.Open(path)
	if err != nil {
		return nil, fmt.Errorf("Invalid file %s: %w", path, err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Failed to close open file %s: %v", path, err)
		}
	}()

	ext := filepath.Ext(path)

	var patch Patch
	switch ext {
	case fileExtJson:
		if err := json.NewDecoder(file).Decode(&patch); err != nil {
			return nil, fmt.Errorf("Invalid patch file %s: %w", path, err)
		}
	case fileExtYaml, fileExtYml:
		if err := yaml.NewDecoder(file).Decode(&patch); err != nil {
			return nil, fmt.Errorf("Invalid patch file %s: %w", path, err)
		}
	default:
		return nil, fmt.Errorf("%w: %s", ErrInvalidExt, ext)
	}

	return &patch, nil
}
