// Package config 提供多来源配置合并、校验与热更新能力。
package config

// ValueType 描述配置键期望的取值类型。
type ValueType int

const (
	TypeString ValueType = iota
	TypeInt
	TypeFloat
	TypeBool
)

// KeySpec 描述单个配置键的约束。
type KeySpec struct {
	Type      ValueType
	Required  bool
	Min       *float64 // 仅对数值类型生效，nil 表示不限制
	Max       *float64 // 仅对数值类型生效，nil 表示不限制
	Sensitive bool     // 敏感字段在导出与日志中脱敏
}

// Schema 是键名到键约束的映射。
type Schema map[string]KeySpec

// Source 表示一个配置来源，后加载的来源优先级更高。
type Source struct {
	Name   string
	Values map[string]any
}

// Config 是一份合并并通过校验的不可变有效配置。
type Config struct {
	values map[string]any
}
