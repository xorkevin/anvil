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

var (
	ErrInvalidExt = errors.New("Invalid component config extension")
)

type (
	Component struct {
		Dir    string
		Base   string
		Format string
	}
)

func ParseComponent(path string) (*Component, error) {
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
	format := ext
	base := filepath.Dir(path)
	dir := filepath.Dir(path)

	var config map[string]interface{}
	switch ext {
	case fileExtJson:
		if err := json.NewDecoder(file).Decode(&config); err != nil {
			return nil, fmt.Errorf("Invalid component config %s: %w", path, err)
		}
	case fileExtYaml, fileExtYml:
		format = fileExtYaml
		if err := yaml.NewDecoder(file).Decode(&config); err != nil {
			return nil, fmt.Errorf("Invalid component config %s: %w", path, err)
		}
	default:
		return nil, fmt.Errorf("%w: %s", ErrInvalidExt, ext)
	}

	return &Component{
		Dir:    dir,
		Base:   base,
		Format: format,
	}, nil
}
