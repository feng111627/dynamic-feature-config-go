// Package feature 提供功能开关与灰度规则。
package feature

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Rule 描述一条灰度规则。
type Rule struct {
	Percentage int      `json:"percentage"` // 0-100，按用户哈希放量
	AllowUsers []string `json:"allowUsers,omitempty"`
}

// Flag 是一个功能开关。
type Flag struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Rule    Rule   `json:"rule"`
}

// ParseFlags 严格解析开关列表：
//   - 未知字段 -> 整体拒绝（DisallowUnknownFields）
//   - percentage 越界（非 0-100）-> 整体拒绝
//   - 开关名为空或重复 -> 整体拒绝
//
// 任何一条非法都返回错误，绝不部分生效。
func ParseFlags(data []byte) ([]Flag, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var flags []Flag
	if err := dec.Decode(&flags); err != nil {
		return nil, fmt.Errorf("feature: parse flags: %w", err)
	}
	seen := make(map[string]struct{}, len(flags))
	for i, f := range flags {
		if f.Name == "" {
			return nil, fmt.Errorf("feature: flag #%d: empty name", i)
		}
		if _, dup := seen[f.Name]; dup {
			return nil, fmt.Errorf("feature: flag %q: duplicated name", f.Name)
		}
		seen[f.Name] = struct{}{}
		if f.Rule.Percentage < 0 || f.Rule.Percentage > 100 {
			return nil, fmt.Errorf("feature: flag %q: percentage %d out of range [0,100]", f.Name, f.Rule.Percentage)
		}
	}
	return flags, nil
}
