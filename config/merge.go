package config

import (
	"fmt"
	"math"
	"sort"
)

// maskedValue 是脱敏后的占位值。
const maskedValue = "***"

// Merge 按优先级从低到高合并多个来源的值，高优先级覆盖低优先级。
// 任一来源加载失败都会返回带 ReasonSourceLoad 的错误，不产生部分合并结果。
func Merge(sources []Source) (map[string]any, error) {
	sorted := make([]Source, len(sources))
	copy(sorted, sources)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Priority() < sorted[j].Priority()
	})

	merged := make(map[string]any)
	for _, src := range sorted {
		vals, err := src.Load()
		if err != nil {
			return nil, &ValidationError{
				Reason: ReasonSourceLoad,
				Key:    src.Name(),
				Detail: err.Error(),
			}
		}
		for k, v := range vals {
			merged[k] = v
		}
	}
	return merged, nil
}

// Validate 依据 Schema 校验合并后的配置。
// 键缺失、类型不匹配、取值越界、出现未知键都会返回可区分的 *ValidationError。
func Validate(values map[string]any, schema Schema) error {
	for key, spec := range schema {
		v, ok := values[key]
		if !ok {
			if spec.Required {
				return &ValidationError{Reason: ReasonMissingKey, Key: key, Detail: "required key is absent"}
			}
			continue
		}
		if err := checkType(key, v, spec); err != nil {
			return err
		}
		if err := checkRange(key, v, spec); err != nil {
			return err
		}
	}
	for key := range values {
		if _, ok := schema[key]; !ok {
			return &ValidationError{Reason: ReasonUnknownField, Key: key, Detail: "key is not declared in schema"}
		}
	}
	return nil
}

func checkType(key string, v any, spec KeySpec) error {
	if spec.Type == "" {
		return nil
	}
	ok := false
	switch spec.Type {
	case "string":
		_, ok = v.(string)
	case "bool":
		_, ok = v.(bool)
	case "int":
		if f, isFloat := v.(float64); isFloat && f == math.Trunc(f) && !math.IsInf(f, 0) {
			ok = true
		}
		_, isInt := v.(int)
		ok = ok || isInt
	case "float64":
		switch v.(type) {
		case float64, int:
			ok = true
		}
	default:
		return &ValidationError{Reason: ReasonTypeMismatch, Key: key,
			Detail: fmt.Sprintf("unsupported spec type %q", spec.Type)}
	}
	if !ok {
		return &ValidationError{Reason: ReasonTypeMismatch, Key: key,
			Detail: fmt.Sprintf("want %s, got %T", spec.Type, v)}
	}
	return nil
}

func checkRange(key string, v any, spec KeySpec) error {
	if spec.Min == nil && spec.Max == nil {
		return nil
	}
	var f float64
	switch n := v.(type) {
	case float64:
		f = n
	case int:
		f = float64(n)
	default:
		return nil // 非数值类型不做范围校验
	}
	if spec.Min != nil && f < *spec.Min {
		return &ValidationError{Reason: ReasonOutOfRange, Key: key,
			Detail: fmt.Sprintf("value %v below min %v", f, *spec.Min)}
	}
	if spec.Max != nil && f > *spec.Max {
		return &ValidationError{Reason: ReasonOutOfRange, Key: key,
			Detail: fmt.Sprintf("value %v above max %v", f, *spec.Max)}
	}
	return nil
}

// Masked 返回脱敏后的副本，敏感键的值被替换为 "***"。
func Masked(values map[string]any, schema Schema) map[string]any {
	out := make(map[string]any, len(values))
	for k, v := range values {
		if spec, ok := schema[k]; ok && spec.Sensitive {
			out[k] = maskedValue
			continue
		}
		out[k] = v
	}
	return out
}
