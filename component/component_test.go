package component

import (
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseConfigFile(t *testing.T) {
	t.Parallel()

	tabReplacer := strings.NewReplacer("\t", "  ")

	for _, tc := range []struct {
		Name           string
		ConfigPath     string
		ConfigData     string
		ComponentsPath string
		ComponentsData string
		Config         Config
	}{
		{
			Name:       "full",
			ConfigPath: "config.yaml",
			ConfigData: `
vars:
	field1:
		field1sub1: hello, world

templates:
	file1:
		path: file1.yaml
		output: file1out.yaml

components: components.yaml
`,
			ComponentsPath: "components.yaml",
			ComponentsData: `
components:
	comp1:
		kind: local
		path: subcomp/config.yaml
		vars:
			field1:
				field1sub1: some val
				field1sub2: {{ .Vars.field1.field1sub1 }}
`,
			Config: Config{
				Name: ".",
				ConfigData: ConfigData{
					Vars: map[string]interface{}{
						"field1": map[string]interface{}{
							"field1sub1": "hello, world",
						},
					},
					Templates: map[string]Template{
						"file1": {
							Path:   "file1.yaml",
							Output: "file1out.yaml",
						},
					},
					Components: "components.yaml",
				},
			},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			assert := require.New(t)

			now := time.Now()
			fsys := fstest.MapFS{
				tc.ConfigPath: &fstest.MapFile{
					Data:    []byte(tabReplacer.Replace(tc.ConfigData)),
					Mode:    0644,
					ModTime: now,
				},
				tc.ComponentsPath: &fstest.MapFile{
					Data:    []byte(tabReplacer.Replace(tc.ComponentsData)),
					Mode:    0644,
					ModTime: now,
				},
			}

			config, err := ParseConfigFile(fsys, tc.ConfigPath)
			assert.NoError(err)
			assert.NotNil(config)
			assert.Equal(tc.Config.Name, config.Name)
			assert.Equal(tc.Config.ConfigData, config.ConfigData)
		})
	}
}

func TestParsePatchFile(t *testing.T) {
	t.Parallel()

	tabReplacer := strings.NewReplacer("\t", "  ")

	for _, tc := range []struct {
		Name  string
		Path  string
		Data  string
		Patch Patch
	}{
		{
			Name: "full",
			Path: "patch.yaml",
			Data: `
vars:
	field1:
		field1sub1: hello, world
templates:
	file1:
		path: file1.yaml
		output: file1out.yaml
components:
	comp1:
		vars:
			field2:
				field2sub1: some val
				field2sub2: other val
		templates:
			file2:
				path: file2.yaml
				output: file2out.yaml
`,
			Patch: Patch{
				Vars: map[string]interface{}{
					"field1": map[string]interface{}{
						"field1sub1": "hello, world",
					},
				},
				Templates: map[string]Template{
					"file1": {
						Path:   "file1.yaml",
						Output: "file1out.yaml",
					},
				},
				Components: map[string]Patch{
					"comp1": {
						Vars: map[string]interface{}{
							"field2": map[string]interface{}{
								"field2sub1": "some val",
								"field2sub2": "other val",
							},
						},
						Templates: map[string]Template{
							"file2": {
								Path:   "file2.yaml",
								Output: "file2out.yaml",
							},
						},
					},
				},
			},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			assert := require.New(t)

			now := time.Now()
			fsys := fstest.MapFS{
				tc.Path: &fstest.MapFile{
					Data:    []byte(tabReplacer.Replace(tc.Data)),
					Mode:    0644,
					ModTime: now,
				},
			}

			patch, err := ParsePatchFile(fsys, tc.Path)
			assert.NoError(err)
			assert.NotNil(patch)
			assert.Equal(tc.Patch, *patch)
		})
	}
}
