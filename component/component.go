package component

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"path/filepath"

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
	ErrInvalidExt = errors.New("Invalid component config extension")
	ErrVarsPatch  = errors.New("Invalid component vars patch")
)

type (
	ConfigData struct {
		Vars       map[string]interface{} `json:"vars" yaml:"vars"`
		Templates  map[string]Template    `json:"templates" yaml:"templates"`
		Components string                 `json:"components" yaml:"components"`
	}

	Template struct {
		Path   string `json:"path" yaml:"path"`
		Output string `json:"output" yaml:"output"`
	}

	Config struct {
		Dir        string
		Base       string
		ConfigData ConfigData
	}

	Component struct {
		Kind string                 `json:"kind" yaml:"kind"`
		Path string                 `json:"path" yaml:"path"`
		Vars map[string]interface{} `json:"vars" yaml:"vars"`
	}

	Patch struct {
		Vars       map[string]interface{} `json:"vars" yaml:"vars"`
		Templates  map[string]Template    `json:"templates" yaml:"templates"`
		Components map[string]Patch       `json:"components" yaml:"components"`
	}
)

func ParseConfigFile(rootdir fs.FS, path string) (*Config, error) {
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
	base := filepath.Base(path)
	dir := filepath.Dir(path)

	var config ConfigData
	switch ext {
	case fileExtJson:
		if err := json.NewDecoder(file).Decode(&config); err != nil {
			return nil, fmt.Errorf("Invalid component config %s: %w", path, err)
		}
	case fileExtYaml, fileExtYml:
		if err := yaml.NewDecoder(file).Decode(&config); err != nil {
			return nil, fmt.Errorf("Invalid component config %s: %w", path, err)
		}
	default:
		return nil, fmt.Errorf("%w: %s", ErrInvalidExt, ext)
	}

	return &Config{
		Dir:        dir,
		Base:       base,
		ConfigData: config,
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
