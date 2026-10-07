package config

import "fmt"

// Merge 按来源顺序合并配置（后者覆盖前者），并依据 Schema 校验。
// 校验规则：
//   - 来源中出现 Schema 未定义的键 -> ReasonUnknownKey
//   - 取值类型与 Schema 不符      -> ReasonTypeMismatch
//   - 数值越界                    -> ReasonOutOfRange
//   - 合并后缺少 Required 键      -> ReasonMissingKey
//
// 任一拒绝项出现即整体失败，返回全部拒绝原因，调用方不得部分套用。
func Merge(schema Schema, sources ...Source) (*Config, []Rejection) {
	var rejections []Rejection
	merged := make(map[string]any, len(schema))

	for _, src := range sources {
		for key, val := range src.Values {
			spec, ok := schema[key]
			if !ok {
				rejections = append(rejections, Rejection{
					Source: src.Name, Key: key, Reason: ReasonUnknownKey,
					Detail: "key is not defined in schema",
				})
				continue
			}
			if r, ok := checkValue(src.Name, key, val, spec); !ok {
				rejections = append(rejections, r)
				continue
			}
			merged[key] = normalize(val)
		}
	}

	for key, spec := range schema {
		if spec.Required {
			if _, ok := merged[key]; !ok {
				rejections = append(rejections, Rejection{
					Source: "", Key: key, Reason: ReasonMissingKey,
					Detail: "required key is absent from all sources",
				})
			}
		}
	}

	if len(rejections) > 0 {
		return nil, rejections
	}
	return &Config{values: merged}, nil
}

// checkValue 校验单个取值，返回拒绝项与是否通过。
func checkValue(source, key string, val any, spec KeySpec) (Rejection, bool) {
	bad := func(detail string) (Rejection, bool) {
		return Rejection{Source: source, Key: key, Reason: ReasonTypeMismatch, Detail: detail}, false
	}
	switch spec.Type {
	case TypeString:
		if _, ok := val.(string); !ok {
			return bad(fmt.Sprintf("want string, got %T", val))
		}
	case TypeBool:
		if _, ok := val.(bool); !ok {
			return bad(fmt.Sprintf("want bool, got %T", val))
		}
	case TypeInt:
		if _, ok := asFloat(val); !ok {
			return bad(fmt.Sprintf("want int, got %T", val))
		}
		f, _ := asFloat(val)
		if f != float64(int64(f)) {
			return bad(fmt.Sprintf("want int, got non-integral %v", val))
		}
		if r, ok := checkRange(source, key, f, spec); !ok {
			return r, false
		}
	case TypeFloat:
		f, ok := asFloat(val)
		if !ok {
			return bad(fmt.Sprintf("want float, got %T", val))
		}
		if r, ok := checkRange(source, key, f, spec); !ok {
			return r, false
		}
	}
	return Rejection{}, true
}

func checkRange(source, key string, f float64, spec KeySpec) (Rejection, bool) {
	if spec.Min != nil && f < *spec.Min {
		return Rejection{Source: source, Key: key, Reason: ReasonOutOfRange,
			Detail: fmt.Sprintf("value %v below min %v", f, *spec.Min)}, false
	}
	if spec.Max != nil && f > *spec.Max {
		return Rejection{Source: source, Key: key, Reason: ReasonOutOfRange,
			Detail: fmt.Sprintf("value %v above max %v", f, *spec.Max)}, false
	}
	return Rejection{}, true
}

// asFloat 把数值类型统一转成 float64 便于比较。
func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}

// normalize 把 int64 统一为 int，保证 Get 返回类型稳定。
func normalize(v any) any {
	if n, ok := v.(int64); ok {
		return int(n)
	}
	return v
}
