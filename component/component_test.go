package component

import (
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseConfigFile(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		Name       string
		Path       string
		Config     string
		ConfigFile ConfigFile
	}{
		{
			Name: "full",
			Path: "config.yaml",
			Config: `
vars:
  field1:
    field1sub1: hello, world

components:
  comp1:
    kind: local
    path: ./subcomp/config.yaml
    vars:
      field1:
        field1sub1: some val
`,
			ConfigFile: ConfigFile{
				Dir:  ".",
				Base: "config.yaml",
				Config: Config{
					Vars:       map[string]interface{}{},
					Components: map[string]Component{},
				},
			},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			assert := require.New(t)

			fsys := fstest.MapFS{
				tc.Path: &fstest.MapFile{
					Data:    []byte(tc.Config),
					Mode:    0644,
					ModTime: time.Now(),
				},
			}

			config, err := ParseConfigFile(fsys, tc.Path, nil)
			assert.NoError(err)
			assert.NotNil(config)
			assert.Equal(tc.ConfigFile, *config)
		})
	}
}
