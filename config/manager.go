package config

import (
	"errors"
	"sync/atomic"
)

// Manager 持有当前生效配置，支持并发安全的热更新。
// 更新整体替换快照，读取方要么看到旧配置要么看到新配置，不会看到混合状态。
type Manager struct {
	sources []Source
	schema  Schema
	current atomic.Value // 存储 map[string]any，写入后只读
}

// NewManager 创建管理器并立即加载一次配置；首次加载失败则返回错误。
func NewManager(sources []Source, schema Schema) (*Manager, error) {
	m := &Manager{sources: sources, schema: schema}
	if err := m.Reload(); err != nil {
		return nil, err
	}
	return m, nil
}

// Reload 重新加载并校验所有来源；失败时保留旧配置并返回错误。
func (m *Manager) Reload() error {
	merged, err := Merge(m.sources)
	if err != nil {
		return err
	}
	if err := Validate(merged, m.schema); err != nil {
		return err
	}
	m.current.Store(merged)
	return nil
}

// ErrNotLoaded 表示尚未成功加载过任何配置。
var ErrNotLoaded = errors.New("config: no configuration loaded")

func (m *Manager) load() map[string]any {
	if v := m.current.Load(); v != nil {
		return v.(map[string]any)
	}
	return nil
}

// Get 返回键对应的值与是否存在。
func (m *Manager) Get(key string) (any, bool) {
	cur := m.load()
	if cur == nil {
		return nil, false
	}
	v, ok := cur[key]
	return v, ok
}

// Snapshot 返回当前生效配置的脱敏副本，可安全用于日志与导出。
func (m *Manager) Snapshot() map[string]any {
	cur := m.load()
	if cur == nil {
		return map[string]any{}
	}
	return Masked(cur, m.schema)
}

// Raw 返回当前生效配置的只读视图（未脱敏，仅供内部使用，调用方不得修改）。
func (m *Manager) Raw() map[string]any {
	return m.load()
}
