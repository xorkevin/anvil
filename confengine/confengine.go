package confengine

import (
	"path"

	"xorkevin.dev/kerrors"
)

var (
	// ErrorUnknownExt is returned when the ext is not supported
	ErrorUnknownExt errUnknownExt

	// ErrorInvalidArgs is returned when calling a function with invalid args
	ErrorInvalidArgs errInvalidArgs
)

type (
	errUnknownExt  struct{}
	errInvalidArgs struct{}
)

func (e errUnknownExt) Error() string {
	return "Unknown config ext"
}

func (e errInvalidArgs) Error() string {
	return "Invalid args"
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
		return nil, ErrorUnknownExt
	}
	b, err := e.Exec(name, args)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate config")
	}
	return b, nil
}
