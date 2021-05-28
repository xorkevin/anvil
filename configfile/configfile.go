package configfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	FileExtJson = ".json"
	FileExtYaml = ".yaml"
	FileExtYml  = ".yml"
)

var (
	// ErrInvalidExt is returned when attempting to parse a file with an invalid extension
	ErrInvalidExt = errors.New("Invalid config extension")
)

// DecodeJSONorYAML decodes json or yaml from an io.Reader
func DecodeJSONorYAML(r io.Reader, ext string, target interface{}) error {
	switch ext {
	case FileExtJson:
		if err := json.NewDecoder(r).Decode(target); err != nil {
			return fmt.Errorf("Invalid JSON: %w", err)
		}
	case FileExtYaml, FileExtYml:
		if err := yaml.NewDecoder(r).Decode(target); err != nil {
			return fmt.Errorf("Invalid YAML: %w", err)
		}
	default:
		return fmt.Errorf("%w: %s", ErrInvalidExt, ext)
	}
	return nil
}

// DecodeJSONorYAMLFile decodes a json or yaml file from a fs.FS
func DecodeJSONorYAMLFile(fsys fs.FS, path string, target interface{}) error {
	file, err := fsys.Open(path)
	if err != nil {
		return fmt.Errorf("Invalid file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Failed to close open file %s: %v", path, err)
		}
	}()
	return DecodeJSONorYAML(file, filepath.Ext(path), target)
}
