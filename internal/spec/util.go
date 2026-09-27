package spec

import (
	"fmt"
	"math"
)

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int64:
		return int(n), true
	case int:
		return n, true
	case float64:
		if n == math.Trunc(n) {
			return int(n), true
		}
	}
	return 0, false
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}

func toRange(v any) ([2]int, error) {
	if n, ok := toInt(v); ok {
		return [2]int{n, n}, nil
	}
	arr, ok := v.([]any)
	if !ok || len(arr) != 2 {
		return [2]int{}, fmt.Errorf("want [a, b]")
	}
	a, ok1 := toInt(arr[0])
	b, ok2 := toInt(arr[1])
	if !ok1 || !ok2 {
		return [2]int{}, fmt.Errorf("want integers")
	}
	return [2]int{a, b}, nil
}

// toStrings accepts a string or a list of strings.
func toStrings(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return x
	}
	return nil
}

func getBool(m map[string]any, key string, def bool) (bool, error) {
	v, ok := m[key]
	if !ok {
		return def, nil
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("%s must be true or false", key)
	}
	return b, nil
}

func tables(v any) []map[string]any {
	switch x := v.(type) {
	case []map[string]any:
		return x
	case []any:
		var out []map[string]any
		for _, e := range x {
			if m, ok := e.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	case map[string]any:
		return []map[string]any{x}
	}
	return nil
}

// deepCopy copies a decoded TOML value so merges never alias.
func deepCopy(v any) any {
	switch x := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(x))
		for k, e := range x {
			m[k] = deepCopy(e)
		}
		return m
	case []map[string]any:
		out := make([]map[string]any, len(x))
		for i, e := range x {
			out[i] = deepCopy(e).(map[string]any)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = deepCopy(e)
		}
		return out
	}
	return v
}
