package confengine

import (
	"path"

	"xorkevin.dev/anvil/util/kjson"
	"xorkevin.dev/kerrors"
)

var (
	// ErrorUnknownExt is returned when the ext is not supported
	ErrorUnknownExt errUnknownExt

	// ErrorInvalidArgs is returned when calling a function with invalid args
	ErrorInvalidArgs errInvalidArgs
)

type (
	errUnknownExt struct{}
)

func (e errUnknownExt) Error() string {
	return "Unknown config ext"
}

type (
	errInvalidArgs struct{}
)

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

type (
	Function    = func(args []any) (any, error)
	FunctionDef struct {
		Function Function
		Params   []string
	}
	Functions = map[string]FunctionDef
)

var DefaultFunctions = Functions{
	"JSONMarshal": FunctionDef{
		Function: func(args []any) (any, error) {
			if len(args) != 1 {
				return nil, kerrors.WithKind(nil, ErrorInvalidArgs, "JSONMarshal needs 1 argument")
			}
			b, err := kjson.Marshal(args[0])
			if err != nil {
				return nil, kerrors.WithMsg(err, "Failed to marshal json")
			}
			return string(b), nil
		},
		Params: []string{"v"},
	},
	"JSONMergePatch": FunctionDef{
		Function: func(args []any) (any, error) {
			if len(args) != 2 {
				return nil, kerrors.WithKind(nil, ErrorInvalidArgs, "JSONMergePatch needs 2 arguments")
			}
			return kjson.MergePatch(args[0], args[1]), nil
		},
		Params: []string{"a", "b"},
	},
}
