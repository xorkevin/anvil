package kjson

import (
	"bytes"
	"encoding/json"
)

// Marshal marshals json without escaping html
func Marshal(v any) ([]byte, error) {
	b := bytes.Buffer{}
	j := json.NewEncoder(&b)
	j.SetEscapeHTML(false)
	if err := j.Encode(v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Unmarshal is [json.Unmarshal]
func Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func MergePatch(target, patch any) any {
	p, ok := patch.(map[string]any)
	if !ok {
		return patch
	}
	t := map[string]any{}
	if ot, ok := target.(map[string]any); ok {
		for k, v := range ot {
			t[k] = v
		}
	}
	for k, v := range p {
		if v == nil {
			delete(t, k)
		} else {
			t[k] = MergePatch(t[k], v)
		}
	}
	return t
}

func MergePatchObj(target, patch map[string]any) map[string]any {
	if len(patch) == 0 {
		return target
	}
	merged, ok := MergePatch(target, patch).(map[string]any)
	if !ok {
		return target
	}
	return merged
}
