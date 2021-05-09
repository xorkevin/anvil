package component

import (
	"context"
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
	now := time.Now()
	var filemode fs.FileMode = 0644

	for _, tc := range []struct {
		Name       string
		LocalFS    fs.FS
		ConfigPath string
		PatchPath  string
		Files      map[string]string
	}{
		{
			Name: "full",
			LocalFS: fstest.MapFS{
				"comp/config.yaml": &fstest.MapFile{
					Data: []byte(tabReplacer.Replace(`
version: xorkevin.dev/anvil/v1alpha1

vars:
	field1:
		field1sub1: hello, world
		field1sub2: out.yaml

configtpl: configtpl.yaml
`)),
					Mode:    filemode,
					ModTime: now,
				},
				"comp/configtpl.yaml": &fstest.MapFile{
					Data: []byte(tabReplacer.Replace(`
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
					)),
					Mode:    filemode,
					ModTime: now,
				},
				"comp/file1.yaml": &fstest.MapFile{
					Data: []byte(tabReplacer.Replace(
						`
file1content: {{ .Vars.field1.field1sub1 }}
`,
					)),
					Mode:    filemode,
					ModTime: now,
				},
				"subcomp/config.yaml": &fstest.MapFile{
					Data: []byte(tabReplacer.Replace(`
version: xorkevin.dev/anvil/v1alpha1

vars:
	field1:
		field1sub1: hello, world
		field1sub2: lorem ipsum

configtpl: configtpl.yaml
`,
					)),
					Mode:    filemode,
					ModTime: now,
				},
				"subcomp/configtpl.yaml": &fstest.MapFile{
					Data: []byte(tabReplacer.Replace(`
templates:
	file1:
		path: file1.yaml
		output: subout.yaml
`,
					)),
					Mode:    filemode,
					ModTime: now,
				},
				"subcomp/file1.yaml": &fstest.MapFile{
					Data: []byte(tabReplacer.Replace(`
file1field1: {{ .Vars.field1.field1sub1 }}
file1field2: {{ .Vars.field1.field1sub2 }}
`,
					)),
					Mode:    filemode,
					ModTime: now,
				},
			},
			ConfigPath: "comp/config.yaml",
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

			writefs := NewWriteFSMock()
			assert.NoError(GenerateComponents(context.Background(), writefs, tc.LocalFS, nil, tc.ConfigPath, tc.PatchPath))
			assert.Equal(tc.Files, writefs.Files)
		})
	}
}
