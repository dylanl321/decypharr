package server

import "fmt"

// templateDict builds a map for use in templates: {{ template "x" (dict "k" v) }}.
func templateDict(values ...interface{}) (map[string]interface{}, error) {
	if len(values)%2 != 0 {
		return nil, fmt.Errorf("dict: expected even number of arguments")
	}
	m := make(map[string]interface{}, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: keys must be strings")
		}
		m[key] = values[i+1]
	}
	return m, nil
}
