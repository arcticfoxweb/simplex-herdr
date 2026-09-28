// Package jutil reads fields out of SimpleX JSON without binding the whole schema.
package jutil

import (
	"encoding/json"
	"fmt"
	"strconv"
)

func Obj(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func Type(m map[string]any) string {
	if m == nil {
		return ""
	}
	s, _ := m["type"].(string)
	return s
}

func Str(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	switch v := m[key].(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func Int(m map[string]any, key string) int64 {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	default:
		return 0
	}
}

func Slice(v any) []any {
	s, _ := v.([]any)
	return s
}
