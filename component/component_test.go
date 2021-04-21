package component

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfigFile(t *testing.T) {
	t.Parallel()

	tabReplacer := strings.NewReplacer("\t", "  ")

	for _, tc := range []struct {
		Name          string
		ConfigPath    string
		ConfigData    string
		ConfigTplPath string
		ConfigTplData string
		TplPath       string
		TplData       string
		ConfigFile    ConfigFile
		Patch         *Patch
		Config        Config
		Files         map[string]string
	}{
		{
			Name:       "full",
			ConfigPath: "config.yaml",
			ConfigData: `
vars:
	field1:
		field1sub1: hello, world
		field1sub2: out.yaml

configtpl: configtpl.yaml
`,
			ConfigTplPath: "configtpl.yaml",
			ConfigTplData: `
templates:
	file1:
		path: file1.yaml
		output: {{ .Vars.field1.field1sub2 }}

components:
	comp1:
		kind: local
		path: subcomp/config.yaml
		vars:
			field1:
				field1sub1: some val
				field1sub2: {{ .Vars.field1.field1sub1 }}
`,
			TplPath: "file1.yaml",
			TplData: `
file1content: {{ .Vars.field1.field1sub1 }}
`,
			ConfigFile: ConfigFile{
				Name: ".",
				ConfigData: ConfigData{
					Vars: map[string]interface{}{
						"field1": map[string]interface{}{
							"field1sub1": "hello, world",
							"field1sub2": "out.yaml",
						},
					},
					ConfigTpl: "configtpl.yaml",
				},
			},
			Patch: nil,
			Config: Config{
				Vars: map[string]interface{}{
					"field1": map[string]interface{}{
						"field1sub1": "hello, world",
						"field1sub2": "out.yaml",
					},
				},
				Templates: map[string]Template{
					"file1": {
						Output: "out.yaml",
					},
				},
				Components: map[string]Component{
					"comp1": {
						Kind: "local",
						Path: "subcomp/config.yaml",
						Vars: map[string]interface{}{
							"field1": map[string]interface{}{
								"field1sub1": "some val",
								"field1sub2": "hello, world",
							},
						},
					},
				},
			},
			Files: map[string]string{
				"out.yaml": `
file1content: hello, world
`,
			},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			assert := require.New(t)

			now := time.Now()
			var filemode fs.FileMode = 0644
			fsys := fstest.MapFS{
				tc.ConfigPath: &fstest.MapFile{
					Data:    []byte(tabReplacer.Replace(tc.ConfigData)),
					Mode:    filemode,
					ModTime: now,
				},
				tc.ConfigTplPath: &fstest.MapFile{
					Data:    []byte(tabReplacer.Replace(tc.ConfigTplData)),
					Mode:    filemode,
					ModTime: now,
				},
				tc.TplPath: &fstest.MapFile{
					Data:    []byte(tabReplacer.Replace(tc.TplData)),
					Mode:    filemode,
					ModTime: now,
				},
			}

			configFile, err := ParseConfigFile(fsys, tc.ConfigPath)
			assert.NoError(err)
			assert.NotNil(configFile)
			assert.Equal(tc.ConfigFile.Name, configFile.Name)
			assert.Equal(tc.ConfigFile.ConfigData, configFile.ConfigData)
			assert.NotNil(configFile.Dir)
			assert.NotNil(configFile.ConfigTpl)

			writefs := NewWriteFSMock()
			config, err := configFile.InitConfig(tc.Patch)
			assert.NoError(err)
			assert.Equal(tc.Config.Vars, config.Vars)
			assert.Len(config.Templates, len(tc.Config.Templates))
			for k, v := range config.Templates {
				assert.Equal(tc.Config.Templates[k].Output, v.Output)
			}
			assert.Equal(tc.Config.Components, config.Components)
			assert.NoError(config.Generate(writefs))
			assert.Equal(tc.Files, writefs.Files)
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
				Templates: map[string]TemplateData{
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
						Templates: map[string]TemplateData{
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
