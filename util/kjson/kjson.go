package kjson

import (
	"bytes"
	"encoding/json"
	"errors"
)

// Marshal marshals json without escaping html
func Marshal(v any) ([]byte, error) {
	var b bytes.Buffer
	j := json.NewEncoder(&b)
	j.SetEscapeHTML(false)
	if err := j.Encode(v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Unmarshal unmarshals json with the option UseNumber
func Unmarshal(data []byte, v any) error {
	if !json.Valid(data) {
		return errors.New("Invalid json")
	}
	j := json.NewDecoder(bytes.NewReader(data))
	j.UseNumber()
	if err := j.Decode(v); err != nil {
		return err
	}
	return nil
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
