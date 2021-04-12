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
	fileExtJson = "json"
	fileExtYaml = "yaml"
	fileExtYml  = "yml"
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
	Config struct {
		Vars       map[string]interface{} `json:"vars" yaml:"vars"`
		Components map[string]Component   `json:"components" yaml:"components"`
	}

	Component struct {
		Kind string                 `json:"kind" yaml:"kind"`
		Path string                 `json:"path" yaml:"path"`
		Vars map[string]interface{} `json:"vars" yaml:"vars"`
	}

	ConfigFile struct {
		Dir    string
		Base   string
		Config Config
	}
)

func ParsePatchFile(rootdir fs.FS, path string) (map[string]interface{}, error) {
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

	var config map[string]interface{}
	switch ext {
	case fileExtJson:
		if err := json.NewDecoder(file).Decode(&config); err != nil {
			return nil, fmt.Errorf("Invalid patch file %s: %w", path, err)
		}
	case fileExtYaml, fileExtYml:
		if err := yaml.NewDecoder(file).Decode(&config); err != nil {
			return nil, fmt.Errorf("Invalid patch file %s: %w", path, err)
		}
	default:
		return nil, fmt.Errorf("%w: %s", ErrInvalidExt, ext)
	}

	return config, nil
}

func ParseConfigFile(rootdir fs.FS, path string, patch interface{}) (*ConfigFile, error) {
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
	base := filepath.Dir(path)
	dir := filepath.Dir(path)

	var config Config
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

	if patch != nil {
		var ok bool
		config.Vars, ok = jsonMergePatch(config.Vars, patch).(map[string]interface{})
		if !ok {
			return nil, ErrVarsPatch
		}
	}

	return &ConfigFile{
		Dir:    dir,
		Base:   base,
		Config: config,
	}, nil
}
