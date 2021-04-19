package component

func jsonMergePatch(target, patch interface{}) interface{} {
	p, ok := patch.(map[string]interface{})
	if !ok {
		return patch
	}
	t := map[string]interface{}{}
	if ot, ok := target.(map[string]interface{}); ok {
		for k, v := range ot {
			t[k] = v
		}
	}
	for k, v := range p {
		if v == nil {
			delete(t, k)
		} else {
			t[k] = jsonMergePatch(t[k], v)
		}
	}
	return t
}

func jsonMergePatchObj(target, patch map[string]interface{}) map[string]interface{} {
	if len(patch) == 0 {
		return target
	}
	merged, ok := jsonMergePatch(target, patch).(map[string]interface{})
	if !ok {
		return target
	}
	return merged
}
