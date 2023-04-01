package confengine

import (
	"io/fs"
	"net/url"
	"path"
	"strings"

	"xorkevin.dev/kerrors"
)

var (
	// ErrUnknownExt is returned when the ext is not supported
	ErrUnknownExt errUnknownExt
	// ErrInvalidArgs is returned when calling a function with invalid args
	ErrInvalidArgs errInvalidArgs
	// ErrInvalidEngineSpec is returned when the engine spec is invalid
	ErrInvalidEngineSpec errInvalidEngineSpec
)

type (
	errUnknownExt        struct{}
	errInvalidArgs       struct{}
	errInvalidEngineSpec struct{}
)

func (e errUnknownExt) Error() string {
	return "Unknown config ext"
}

func (e errInvalidArgs) Error() string {
	return "Invalid args"
}

func (e errInvalidEngineSpec) Error() string {
	return "Invalid engine spec"
}

type (
	// ConfEngine is a config engine
	ConfEngine interface {
		Exec(name string, args map[string]any) ([]byte, error)
	}

	// Map is a map from file extensions to conf engines
	Map map[string]ConfEngine
)

// Exec generates config using the conf engine mapped to the file extension
func (m Map) Exec(name string, args map[string]any) ([]byte, error) {
	ext := path.Ext(name)
	e, ok := m[ext]
	if !ok {
		return nil, ErrUnknownExt
	}
	b, err := e.Exec(name, args)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate config")
	}
	return b, nil
}

type (
	// Builder builds a config engine
	Builder interface {
		Build(fsys fs.FS) (ConfEngine, error)
	}

	// Cache is a config engine cache by path
	Cache struct {
		algs  map[string]Builder
		fsys  fs.FS
		cache map[string]ConfEngine
	}
)

// NewCache creates a new [*Cache]
func NewCache(fsys fs.FS) *Cache {
	return &Cache{
		algs:  map[string]Builder{},
		fsys:  fsys,
		cache: map[string]ConfEngine{},
	}
}

// Register registers a config engine builder
func (c *Cache) Register(kind string, b Builder) {
	c.algs[kind] = b
}

func (c *Cache) cacheKey(kind string, dir string) string {
	var s strings.Builder
	s.WriteString(url.QueryEscape(kind))
	s.WriteString(":")
	s.WriteString(url.QueryEscape(dir))
	return s.String()
}

func (c *Cache) Get(kind string, dir string) (ConfEngine, error) {
	build, ok := c.algs[kind]
	if !ok {
		return nil, kerrors.WithKind(nil, ErrInvalidEngineSpec, "Invalid engine kind")
	}
	dir = path.Clean(dir)
	key := c.cacheKey(kind, dir)
	if e, ok := c.cache[key]; ok {
		return e, nil
	}
	// TODO: get fsys from repofetcher
	fsys, err := fs.Sub(c.fsys, dir)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get subdir")
	}
	eng, err := build.Build(fsys)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to build config engine")
	}
	c.cache[key] = eng
	return eng, nil
}
