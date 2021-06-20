package component

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_jsonMergePatch(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		Name     string
		Target   string
		Patch    string
		Expected string
	}{
		{
			Target: `
{
 "a": "b",
 "c": {
   "d": "e",
   "f": "g"
 }
}
`,
			Patch: `
{
 "a":"z",
 "c": {
   "f": null
 }
}
`,
			Expected: `
{
 "a":"z",
 "c": {
   "d": "e"
 }
}
`,
		},
		{
			Target: `
{
  "title": "Goodbye!",
  "author" : {
    "givenName" : "John",
    "familyName" : "Doe"
  },
  "tags":[ "example", "sample" ],
  "content": "This will be unchanged"
}
`,
			Patch: `
{
  "title": "Hello!",
  "phoneNumber": "+01-123-456-7890",
  "author": {
    "familyName": null
  },
  "tags": [ "example" ]
}
`,
			Expected: `
{
  "title": "Hello!",
  "author" : {
    "givenName" : "John"
  },
  "tags": [ "example" ],
  "content": "This will be unchanged",
  "phoneNumber": "+01-123-456-7890"
}
`,
		},
		{
			Target:   `{"a":"b"}`,
			Patch:    `{"a":"c"}`,
			Expected: `{"a":"c"}`,
		},
		{
			Target:   `{"a":"b"}`,
			Patch:    `{"b":"c"}`,
			Expected: `{"a":"b","b":"c"}`,
		},
		{
			Target:   `{"a":"b"}`,
			Patch:    `{"a":null}`,
			Expected: `{}`,
		},
		{
			Target:   `{"a":"b","b":"c"}`,
			Patch:    `{"a":null}`,
			Expected: `{"b":"c"}`,
		},
		{
			Target:   `{"a":["b"]}`,
			Patch:    `{"a":"c"}`,
			Expected: `{"a":"c"}`,
		},
		{
			Target:   `{"a":"c"}`,
			Patch:    `{"a":["b"]}`,
			Expected: `{"a":["b"]}`,
		},
		{
			Target:   `{"a":{"b":"c"}}`,
			Patch:    `{"a":{"b":"d","c":null}}`,
			Expected: `{"a":{"b":"d"}}`,
		},
		{
			Target:   `{"a":[{"b":"c"}]}`,
			Patch:    `{"a":[1]}`,
			Expected: `{"a":[1]}`,
		},
		{
			Target:   `["a","b"]`,
			Patch:    `["c","d"]`,
			Expected: `["c","d"]`,
		},
		{
			Target:   `{"a":"b"}`,
			Patch:    `["c"]`,
			Expected: `["c"]`,
		},
		{
			Target:   `{"a":"foo"}`,
			Patch:    `null`,
			Expected: `null`,
		},
		{
			Target:   `{"a":"foo"}`,
			Patch:    `"bar"`,
			Expected: `"bar"`,
		},
		{
			Target:   `{"e":null}`,
			Patch:    `{"a":1}`,
			Expected: `{"e":null,"a":1}`,
		},
		{
			Target:   `[1,2]`,
			Patch:    `{"a":"b","c":null}`,
			Expected: `{"a":"b"}`,
		},
		{
			Target:   `{}`,
			Patch:    `{"a":{"bb":{"ccc":null}}}`,
			Expected: `{"a":{"bb":{}}}`,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			tc := tc
			t.Parallel()
			assert := require.New(t)

			var target, patch, expected interface{}
			assert.NoError(json.Unmarshal([]byte(tc.Target), &target))
			assert.NoError(json.Unmarshal([]byte(tc.Patch), &patch))
			assert.NoError(json.Unmarshal([]byte(tc.Expected), &expected))
			assert.Equal(expected, jsonMergePatch(target, patch))
		})
	}
}

func Test_jsonMergePatchObj(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		Name     string
		Target   string
		Patch    string
		Expected string
	}{
		{
			Target: `
{
 "a": "b",
 "c": {
   "d": "e",
   "f": "g"
 }
}
`,
			Patch: `
{
 "a":"z",
 "c": {
   "f": null
 }
}
`,
			Expected: `
{
 "a":"z",
 "c": {
   "d": "e"
 }
}
`,
		},
		{
			Target: `
{
  "title": "Goodbye!",
  "author" : {
    "givenName" : "John",
    "familyName" : "Doe"
  },
  "tags":[ "example", "sample" ],
  "content": "This will be unchanged"
}
`,
			Patch: `
{
  "title": "Hello!",
  "phoneNumber": "+01-123-456-7890",
  "author": {
    "familyName": null
  },
  "tags": [ "example" ]
}
`,
			Expected: `
{
  "title": "Hello!",
  "author" : {
    "givenName" : "John"
  },
  "tags": [ "example" ],
  "content": "This will be unchanged",
  "phoneNumber": "+01-123-456-7890"
}
`,
		},
		{
			Target:   `{"a":"b"}`,
			Patch:    `{"a":"c"}`,
			Expected: `{"a":"c"}`,
		},
		{
			Target:   `{"a":"b"}`,
			Patch:    `{"b":"c"}`,
			Expected: `{"a":"b","b":"c"}`,
		},
		{
			Target:   `{"a":"b"}`,
			Patch:    `{"a":null}`,
			Expected: `{}`,
		},
		{
			Target:   `{"a":"b","b":"c"}`,
			Patch:    `{"a":null}`,
			Expected: `{"b":"c"}`,
		},
		{
			Target:   `{"a":["b"]}`,
			Patch:    `{"a":"c"}`,
			Expected: `{"a":"c"}`,
		},
		{
			Target:   `{"a":"c"}`,
			Patch:    `{"a":["b"]}`,
			Expected: `{"a":["b"]}`,
		},
		{
			Target:   `{"a":{"b":"c"}}`,
			Patch:    `{"a":{"b":"d","c":null}}`,
			Expected: `{"a":{"b":"d"}}`,
		},
		{
			Target:   `{"a":[{"b":"c"}]}`,
			Patch:    `{"a":[1]}`,
			Expected: `{"a":[1]}`,
		},
		{
			Target:   `["a","b"]`,
			Patch:    `["c","d"]`,
			Expected: `["c","d"]`,
		},
		{
			Target:   `{"a":"b"}`,
			Patch:    `["c"]`,
			Expected: `["c"]`,
		},
		{
			Target:   `{"a":"foo"}`,
			Patch:    `null`,
			Expected: `{"a":"foo"}`,
		},
		{
			Target:   `{"a":"foo"}`,
			Patch:    `"bar"`,
			Expected: `"bar"`,
		},
		{
			Target:   `{"e":null}`,
			Patch:    `{"a":1}`,
			Expected: `{"e":null,"a":1}`,
		},
		{
			Target:   `[1,2]`,
			Patch:    `{"a":"b","c":null}`,
			Expected: `{"a":"b"}`,
		},
		{
			Target:   `{}`,
			Patch:    `{"a":{"bb":{"ccc":null}}}`,
			Expected: `{"a":{"bb":{}}}`,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			tc := tc
			t.Parallel()
			assert := require.New(t)

			var target, patch, expected map[string]interface{}
			assert.NoError(json.Unmarshal([]byte(tc.Target), &target))
			assert.NoError(json.Unmarshal([]byte(tc.Patch), &patch))
			assert.NoError(json.Unmarshal([]byte(tc.Expected), &expected))
			assert.Equal(expected, jsonMergePatchObj(target, patch))
		})
	}
}
