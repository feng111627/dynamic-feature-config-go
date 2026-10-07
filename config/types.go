// Package config 提供多来源配置合并、校验与热更新能力。
package config

import "fmt"

// RejectReason 区分配置被拒绝的原因。
type RejectReason string

const (
	ReasonMissingKey   RejectReason = "missing_key"
	ReasonTypeMismatch RejectReason = "type_mismatch"
	ReasonOutOfRange   RejectReason = "out_of_range"
	ReasonUnknownField RejectReason = "unknown_field"
	ReasonSourceLoad   RejectReason = "source_load"
)

// ValidationError 描述一次配置校验失败。
type ValidationError struct {
	Reason RejectReason
	Key    string
	Detail string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("config rejected [%s] key=%q: %s", e.Reason, e.Key, e.Detail)
}

// Source 是一个配置来源，Priority 越大优先级越高（后加载覆盖先加载）。
type Source interface {
	Name() string
	Priority() int
	Load() (map[string]any, error)
}

// MapSource 基于内存 map 的配置来源。
type MapSource struct {
	SourceName     string
	SourcePriority int
	Values         map[string]any
}

func (s MapSource) Name() string                  { return s.SourceName }
func (s MapSource) Priority() int                 { return s.SourcePriority }
func (s MapSource) Load() (map[string]any, error) { return s.Values, nil }

// KeySpec 描述一个配置键的约束。
type KeySpec struct {
	Required bool
	// Type 期望的类型："string" "bool" "float64" "int" 等，空表示不校验类型。
	Type string
	// Min/Max 仅对数值类型生效，nil 表示不限制。
	Min *float64
	Max *float64
	// Sensitive 为 true 时对外导出与日志需脱敏。
	Sensitive bool
}

// Schema 是一组键约束。
type Schema map[string]KeySpec
