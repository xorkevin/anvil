package component

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	fileExtJson = "json"
	fileExtYaml = "yaml"
	fileExtYml  = "yml"
)

const (
	configKeyVars = "vars"
)

var (
	ErrInvalidExt = errors.New("Invalid component config extension")
	ErrVarsPatch  = errors.New("Invalid component vars patch")
)

type (
	Component struct {
		Dir    string
		Base   string
		config map[string]interface{}
	}
)

func ParseComponent(path string, patch interface{}) (*Component, error) {
	file, err := os.Open(path)
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

	var config map[string]interface{}
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
		config["vars"], ok = jsonMergePatch(config["vars"], patch).(map[string]interface{})
		if !ok {
			return nil, ErrVarsPatch
		}
	}

	return &Component{
		Dir:    dir,
		Base:   base,
		config: config,
	}, nil
}
