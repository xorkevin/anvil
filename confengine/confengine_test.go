package confengine

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

type (
	mockEngine struct{}
)

func (e mockEngine) Exec(name string, args map[string]any) ([]byte, error) {
	j, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	io.WriteString(&b, name)
	io.WriteString(&b, ": ")
	if _, err := b.Write(j); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func Test_ConfEngine(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		Name     string
		Filename string
		Args     map[string]any
		Expected any
	}{
		{
			Name:     "executes confengine",
			Filename: "foo.mockengine",
			Args: map[string]any{
				"hello": "world",
			},
			Expected: map[string]any{
				"hello": "world",
			},
		},
	} {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			assert := require.New(t)

			engines := Map{
				".mockengine": mockEngine{},
			}
			outbytes, err := engines.Exec(tc.Filename, tc.Args)
			assert.NoError(err)
			assert.True(bytes.HasPrefix(outbytes, []byte(tc.Filename+": ")))
			outbytes = outbytes[len(tc.Filename)+2:]
			var out any
			assert.NoError(json.Unmarshal(outbytes, &out))
			assert.Equal(tc.Expected, out)
		})
	}
}
