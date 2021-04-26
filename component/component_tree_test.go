package component

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestComponentTree(t *testing.T) {
	t.Parallel()

	tabReplacer := strings.NewReplacer("\t", "  ")

	for _, tc := range []struct {
		Name             string
		ConfigPath       string
		ConfigData       string
		ConfigTplPath    string
		ConfigTplData    string
		TplPath          string
		TplData          string
		SubConfigPath    string
		SubConfigData    string
		SubConfigTplPath string
		SubConfigTplData string
		SubTplPath       string
		SubTplData       string
		PatchPath        string
		PatchData        string
		Files            map[string]string
	}{
		{
			Name:       "full",
			ConfigPath: "comp/config.yaml",
			ConfigData: `
version: xorkevin.dev/anvil/v1alpha1

vars:
	field1:
		field1sub1: hello, world
		field1sub2: out.yaml

configtpl: configtpl.yaml
`,
			ConfigTplPath: "comp/configtpl.yaml",
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
			TplPath: "comp/file1.yaml",
			TplData: `
file1content: {{ .Vars.field1.field1sub1 }}
`,
			SubConfigPath: "subcomp/config.yaml",
			SubConfigData: `
version: xorkevin.dev/anvil/v1alpha1

vars:
	field1:
		field1sub1: hello, world
		field1sub2: lorem ipsum

configtpl: configtpl.yaml
`,
			SubConfigTplPath: "subcomp/configtpl.yaml",
			SubConfigTplData: `
templates:
	file1:
		path: file1.yaml
		output: subout.yaml
`,
			SubTplPath: "subcomp/file1.yaml",
			SubTplData: `
file1field1: {{ .Vars.field1.field1sub1 }}
file1field2: {{ .Vars.field1.field1sub2 }}
`,
			Files: map[string]string{
				"out.yaml": `
file1content: hello, world
`,
				"subout.yaml": `
file1field1: some val
file1field2: hello, world
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
				tc.SubConfigPath: &fstest.MapFile{
					Data:    []byte(tabReplacer.Replace(tc.SubConfigData)),
					Mode:    filemode,
					ModTime: now,
				},
				tc.SubConfigTplPath: &fstest.MapFile{
					Data:    []byte(tabReplacer.Replace(tc.SubConfigTplData)),
					Mode:    filemode,
					ModTime: now,
				},
				tc.SubTplPath: &fstest.MapFile{
					Data:    []byte(tabReplacer.Replace(tc.SubTplData)),
					Mode:    filemode,
					ModTime: now,
				},
			}

			writefs := NewWriteFSMock()
			assert.NoError(GenerateComponents(writefs, fsys, nil, tc.ConfigPath, tc.PatchPath))
			assert.Equal(tc.Files, writefs.Files)
		})
	}
}
