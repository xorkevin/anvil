package confengine

import (
	"path"

	"xorkevin.dev/anvil/util/kjson"
	"xorkevin.dev/kerrors"
)

var (
	// ErrorUnknownExt is returned when the ext is not supported
	ErrorUnknownExt errUnknownExt
)

type (
	errUnknownExt struct{}
)

func (e errUnknownExt) Error() string {
	return "Unknown config ext"
}

type (
	Vars = map[string][]byte

	// ConfEngine is a config engine
	ConfEngine interface {
		Exec(name string, vars Vars) ([]byte, error)
	}

	// Map is a map from file extensions to conf engines
	Map map[string]ConfEngine
)

// Exec generates config using the conf engine mapped to the file extension
func (m Map) Exec(name string, env map[string][]byte) ([]byte, error) {
	ext := path.Ext(name)
	e, ok := m[ext]
	if !ok {
		return nil, ErrorUnknownExt
	}
	b, err := e.Exec(name, env)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate config")
	}
	return b, nil
}

type (
	Function    = func(args []interface{}) (interface{}, error)
	FunctionDef struct {
		Function Function
		Params   []string
	}
	Functions = map[string]FunctionDef
)

var (
	DefaultFunctions = Functions{
		"marshalJSON": FunctionDef{
			Function: func(args []interface{}) (interface{}, error) {
				b, err := kjson.Marshal(args[0])
				if err != nil {
					return nil, kerrors.WithMsg(err, "Failed to marshal json")
				}
				return string(b), nil
			},
			Params: []string{"v"},
		},
	}
)
