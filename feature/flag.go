// Package feature 提供功能开关与灰度规则评估。
package feature

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
)

// Rule 描述一条灰度规则。
type Rule struct {
	// Percentage 放量百分比，取值 [0,100]。
	Percentage float64 `json:"percentage"`
	// AllowUsers 白名单用户，直接命中。
	AllowUsers []string `json:"allow_users,omitempty"`
}

// Flag 是一个功能开关。
type Flag struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Rule    *Rule  `json:"rule,omitempty"`
}

// ParseFlags 严格解析开关配置 JSON。
// 出现未知字段、缺失名称或非法比例时整体拒绝，返回错误而不产生部分结果。
func ParseFlags(data []byte) ([]Flag, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var flags []Flag
	if err := dec.Decode(&flags); err != nil {
		return nil, fmt.Errorf("feature: parse flags: %w", err)
	}
	seen := make(map[string]bool, len(flags))
	for i, f := range flags {
		if f.Name == "" {
			return nil, fmt.Errorf("feature: flag #%d has empty name", i)
		}
		if seen[f.Name] {
			return nil, fmt.Errorf("feature: duplicate flag name %q", f.Name)
		}
		seen[f.Name] = true
		if f.Rule != nil && (f.Rule.Percentage < 0 || f.Rule.Percentage > 100) {
			return nil, fmt.Errorf("feature: flag %q percentage %v out of range [0,100]",
				f.Name, f.Rule.Percentage)
		}
	}
	return flags, nil
}

// Evaluator 对一组开关做评估。
type Evaluator struct {
	flags map[string]Flag
}

// NewEvaluator 从开关列表构建评估器。
func NewEvaluator(flags []Flag) *Evaluator {
	m := make(map[string]Flag, len(flags))
	for _, f := range flags {
		m[f.Name] = f
	}
	return &Evaluator{flags: m}
}

// Enabled 判断某开关对指定用户是否开启。
// 百分比放量基于 (开关名, 用户ID) 的稳定哈希分桶，同一用户多次评估结果一致。
func (e *Evaluator) Enabled(flagName, userID string) bool {
	f, ok := e.flags[flagName]
	if !ok || !f.Enabled {
		return false
	}
	if f.Rule == nil {
		return true
	}
	for _, u := range f.Rule.AllowUsers {
		if u == userID {
			return true
		}
	}
	return bucket(flagName, userID) < f.Rule.Percentage
}

// bucket 将 (flag, user) 稳定映射到 [0,100) 的桶值。
func bucket(flagName, userID string) float64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(flagName))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(userID))
	// 取模 10000 得到两位小数的稳定桶值。
	return float64(h.Sum64()%10000) / 100
}
