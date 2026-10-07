package feature

import (
	"errors"
	"hash/fnv"
	"sync/atomic"
)

// ErrInvalidFlags 表示开关配置整体非法。
var ErrInvalidFlags = errors.New("feature: invalid flags")

// snapshot 是不可变的开关集合。
type snapshot struct {
	flags map[string]Flag
}

// Store 持有当前生效的开关集合，支持并发安全的热更新。
// 快照整体原子切换，评估端不会看到半新半旧的混合状态。
type Store struct {
	cur atomic.Pointer[snapshot]
}

// NewStore 构建开关 Store；flagsJSON 非法时整体拒绝。
func NewStore(flagsJSON []byte) (*Store, error) {
	flags, err := ParseFlags(flagsJSON)
	if err != nil {
		return nil, err
	}
	s := &Store{}
	s.cur.Store(newSnapshot(flags))
	return s, nil
}

// Update 热更新开关；非法时保留旧配置并返回错误。
func (s *Store) Update(flagsJSON []byte) error {
	flags, err := ParseFlags(flagsJSON)
	if err != nil {
		return errors.Join(ErrInvalidFlags, err)
	}
	s.cur.Store(newSnapshot(flags))
	return nil
}

// Enabled 评估某用户是否命中开关。
// 评估顺序：开关存在且启用 -> AllowUsers 直接命中 -> 按 (flag, user) 哈希百分比放量。
// 哈希对同一 (flag, user) 稳定，因此同一用户多次评估结果一致。
func (s *Store) Enabled(flagName, userID string) bool {
	f, ok := s.cur.Load().flags[flagName]
	if !ok || !f.Enabled {
		return false
	}
	for _, u := range f.Rule.AllowUsers {
		if u == userID {
			return true
		}
	}
	if f.Rule.Percentage <= 0 {
		return false
	}
	if f.Rule.Percentage >= 100 {
		return true
	}
	return bucket(flagName, userID) < f.Rule.Percentage
}

// bucket 把 (flag, user) 稳定映射到 [0,100) 的桶。
func bucket(flagName, userID string) int {
	h := fnv.New64a()
	h.Write([]byte(flagName))
	h.Write([]byte{0})
	h.Write([]byte(userID))
	return int(h.Sum64() % 100)
}

func newSnapshot(flags []Flag) *snapshot {
	m := make(map[string]Flag, len(flags))
	for _, f := range flags {
		m[f.Name] = f
	}
	return &snapshot{flags: m}
}
