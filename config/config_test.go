package config

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

func fptr(f float64) *float64 { return &f }

func testSchema() Schema {
	return Schema{
		"app.name":    {Required: true, Type: "string"},
		"http.port":   {Required: true, Type: "int", Min: fptr(1), Max: fptr(65535)},
		"debug":       {Type: "bool"},
		"db.password": {Type: "string", Sensitive: true},
		"rate.limit":  {Type: "float64", Min: fptr(0)},
	}
}

func baseSources() []Source {
	return []Source{
		MapSource{SourceName: "defaults", SourcePriority: 0, Values: map[string]any{
			"app.name": "demo", "http.port": float64(8080), "debug": false,
			"db.password": "default-secret", "rate.limit": float64(10),
		}},
		MapSource{SourceName: "local-file", SourcePriority: 10, Values: map[string]any{
			"http.port": float64(9090), "db.password": "file-secret",
		}},
		MapSource{SourceName: "env", SourcePriority: 20, Values: map[string]any{
			"debug": true,
		}},
	}
}

func TestMergePriority(t *testing.T) {
	t.Log("业务: 多来源配置合并 —— 高优先级来源覆盖低优先级来源")
	merged, err := Merge(baseSources())
	if err != nil {
		t.Fatalf("Merge failed: %v", err)
	}
	cases := []struct {
		key  string
		want any
		from string
	}{
		{"app.name", "demo", "defaults(未被覆盖)"},
		{"http.port", float64(9090), "local-file 覆盖 defaults"},
		{"debug", true, "env 覆盖 defaults"},
		{"db.password", "file-secret", "local-file 覆盖 defaults"},
	}
	for _, c := range cases {
		if got := merged[c.key]; got != c.want {
			t.Errorf("key %s: got %v, want %v (%s)", c.key, got, c.want, c.from)
		} else {
			t.Logf("  用例[%s] 通过: %s = %v", c.from, c.key, got)
		}
	}
}

type failSource struct{ name string }

func (s failSource) Name() string                  { return s.name }
func (s failSource) Priority() int                 { return 99 }
func (s failSource) Load() (map[string]any, error) { return nil, errors.New("disk read error") }

func TestRejectReasons(t *testing.T) {
	t.Log("业务: 非法配置拒绝 —— 拒绝原因可区分，且不产生部分合并结果")
	cases := []struct {
		name   string
		mutate func(vals map[string]any)
		source Source
		want   RejectReason
	}{
		{"键缺失", func(v map[string]any) { delete(v, "app.name") }, nil, ReasonMissingKey},
		// http.port 需改高优先级的 local-file 来源，否则会被覆盖而看不到非法值。
		{"类型不匹配", func(v map[string]any) { v["http.port"] = "not-a-number" }, nil, ReasonTypeMismatch},
		{"取值越界", func(v map[string]any) { v["http.port"] = float64(70000) }, nil, ReasonOutOfRange},
		{"未知键", func(v map[string]any) { v["surprise.key"] = 1.0 }, nil, ReasonUnknownField},
		{"来源加载失败", nil, failSource{name: "broken"}, ReasonSourceLoad},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srcs := baseSources()
			if c.mutate != nil {
				if c.want == ReasonMissingKey {
					c.mutate(srcs[0].(MapSource).Values) // app.name 仅 defaults 提供
				} else {
					c.mutate(srcs[1].(MapSource).Values) // http.port 改最高优先级持有者
				}
			}
			if c.source != nil {
				srcs = append(srcs, c.source)
			}
			merged, err := Merge(srcs)
			if err == nil {
				err = Validate(merged, testSchema())
			}
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("expected *ValidationError, got %v", err)
			}
			if ve.Reason != c.want {
				t.Fatalf("reason = %s, want %s (err=%v)", ve.Reason, c.want, ve)
			}
			t.Logf("  用例[%s] 通过: 拒绝原因=%s, 详情=%v", c.name, ve.Reason, ve)
		})
	}
}

func TestReloadFallback(t *testing.T) {
	t.Log("业务: 热更新回退 —— 新配置非法时上一份可用配置继续生效")
	good := MapSource{SourceName: "defaults", SourcePriority: 0, Values: map[string]any{
		"app.name": "demo", "http.port": float64(8080), "db.password": "s1", "rate.limit": float64(5),
	}}
	override := MapSource{SourceName: "override", SourcePriority: 10, Values: map[string]any{}}
	m, err := NewManager([]Source{good, override}, testSchema())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// 推入非法配置（端口越界），Reload 应失败且旧值保留。
	override.Values["http.port"] = float64(-1)
	if err := m.Reload(); err == nil {
		t.Fatal("expected reload to fail")
	} else {
		t.Logf("  非法配置被拒绝: %v", err)
	}
	if v, _ := m.Get("http.port"); v != float64(8080) {
		t.Fatalf("old config not preserved, http.port=%v", v)
	}
	t.Log("  用例[非法配置回退] 通过: http.port 仍为 8080")

	// 修复为合法值后 Reload 应生效。
	override.Values["http.port"] = float64(1234)
	if err := m.Reload(); err != nil {
		t.Fatalf("reload with fixed config failed: %v", err)
	}
	if v, _ := m.Get("http.port"); v != float64(1234) {
		t.Fatalf("new config not applied, http.port=%v", v)
	}
	t.Log("  用例[修复后生效] 通过: http.port 更新为 1234")
}

func TestConcurrentHotReload(t *testing.T) {
	t.Log("业务: 并发热更新 —— 更新期间读取不得出现半新半旧的混合状态")
	base := MapSource{SourceName: "defaults", SourcePriority: 0, Values: map[string]any{
		"app.name": "demo", "http.port": float64(8080), "db.password": "s1", "rate.limit": float64(1),
	}}
	override := MapSource{SourceName: "override", SourcePriority: 10, Values: map[string]any{}}
	m, err := NewManager([]Source{base, override}, testSchema())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// 版本 i: port=8000+i 且 rate.limit=100+i，两者必须同版本出现。
	const versions = 50
	var wg sync.WaitGroup
	stop := make(chan struct{})
	errCh := make(chan error, 16)

	for r := 0; r < 8; r++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				raw := m.Raw()
				port, _ := raw["http.port"].(float64)
				limit, _ := raw["rate.limit"].(float64)
				// 同一份快照内 port 与 limit 的版本号必须一致。
				if port-8000 != limit-100 && !(port == 8080 && limit == 1) {
					errCh <- fmt.Errorf("reader %d saw mixed state: port=%v limit=%v", id, port, limit)
					return
				}
			}
		}(r)
	}

	for i := 0; i < versions; i++ {
		override.Values["http.port"] = float64(8000 + i)
		override.Values["rate.limit"] = float64(100 + i)
		if err := m.Reload(); err != nil {
			t.Fatalf("reload %d failed: %v", i, err)
		}
	}
	close(stop)
	wg.Wait()
	select {
	case err := <-errCh:
		t.Fatal(err)
	default:
	}
	t.Logf("  用例[8 读协程 x %d 次热更新] 通过: 未观察到混合状态", versions)
}

func TestMasking(t *testing.T) {
	t.Log("业务: 敏感字段脱敏 —— 对外导出与日志中敏感值必须被隐藏")
	m, err := NewManager(baseSources(), testSchema())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	snap := m.Snapshot()
	if got := snap["db.password"]; got != "***" {
		t.Fatalf("sensitive key not masked: %v", got)
	}
	if got := snap["app.name"]; got != "demo" {
		t.Fatalf("non-sensitive key altered: %v", got)
	}
	// 脱敏副本的修改不得影响内部配置。
	snap["app.name"] = "hacked"
	if v, _ := m.Get("app.name"); v != "demo" {
		t.Fatalf("snapshot mutation leaked into manager: %v", v)
	}
	t.Log("  用例[敏感键脱敏/非敏感键保留/副本隔离] 通过")
}
