package config

import "sync/atomic"

// mask 是敏感字段对外展示时的掩码。
const mask = "***"

// Store 持有当前生效配置，支持并发安全的热更新。
// 快照通过 atomic.Pointer 整体切换，读取端不会看到半新半旧的混合状态。
type Store struct {
	schema Schema
	cur    atomic.Pointer[Config]
}

// NewStore 用初始来源构建 Store；初始配置非法时返回拒绝原因。
func NewStore(schema Schema, sources ...Source) (*Store, []Rejection) {
	cfg, rej := Merge(schema, sources...)
	if len(rej) > 0 {
		return nil, rej
	}
	s := &Store{schema: schema}
	s.cur.Store(cfg)
	return s, nil
}

// Snapshot 返回当前生效配置的不可变快照。
// 需要一致地读取多个键时，应通过同一份快照读取。
func (s *Store) Snapshot() *Config { return s.cur.Load() }

// Get 读取当前生效配置中的键值（单键读取等价于快照读取）。
func (s *Store) Get(key string) (any, bool) { return s.Snapshot().Get(key) }

// GetString 是 Get 的便捷封装。
func (s *Store) GetString(key string) (string, bool) { return s.Snapshot().GetString(key) }

// GetInt 是 Get 的便捷封装。
func (s *Store) GetInt(key string) (int, bool) { return s.Snapshot().GetInt(key) }

// GetBool 是 Get 的便捷封装。
func (s *Store) GetBool(key string) (bool, bool) { return s.Snapshot().GetBool(key) }

// Update 校验并原子切换到新配置；校验失败时保留旧配置并返回拒绝原因。
func (s *Store) Update(sources ...Source) []Rejection {
	cfg, rej := Merge(s.schema, sources...)
	if len(rej) > 0 {
		return rej
	}
	s.cur.Store(cfg)
	return nil
}

// Export 导出当前配置的脱敏快照，敏感字段被掩码，可安全用于日志与对外接口。
func (s *Store) Export() map[string]any {
	cfg := s.cur.Load()
	out := make(map[string]any, len(cfg.values))
	for k, v := range cfg.values {
		if spec, ok := s.schema[k]; ok && spec.Sensitive {
			out[k] = mask
			continue
		}
		out[k] = v
	}
	return out
}

// ---- Config 的只读访问 ----

// Get 从快照中读取键值。
func (c *Config) Get(key string) (any, bool) {
	v, ok := c.values[key]
	return v, ok
}

// GetString 是 Get 的便捷封装。
func (c *Config) GetString(key string) (string, bool) {
	v, ok := c.Get(key)
	if !ok {
		return "", false
	}
	str, ok := v.(string)
	return str, ok
}

// GetInt 是 Get 的便捷封装。
func (c *Config) GetInt(key string) (int, bool) {
	v, ok := c.Get(key)
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	}
	return 0, false
}

// GetBool 是 Get 的便捷封装。
func (c *Config) GetBool(key string) (bool, bool) {
	v, ok := c.Get(key)
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}
