package config

import (
	"sync"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, rej := NewStore(testSchema(), Source{Name: "file", Values: map[string]any{
		"app.name": "demo", "app.port": 8080, "db.password": "s3cret",
	}})
	if len(rej) != 0 {
		t.Fatalf("init store: %v", rej)
	}
	return s
}

func TestStore_InvalidUpdateKeepsOldConfig(t *testing.T) {
	t.Log("[业务:热更新] 用例:非法更新被拒绝, 上一份可用配置继续生效")
	s := newTestStore(t)
	rej := s.Update(Source{Name: "bad", Values: map[string]any{
		"app.name": "demo", "app.port": 99999, // 越界
	}})
	if len(rej) == 0 {
		t.Fatalf("want rejections for out-of-range port")
	}
	if rej[0].Reason != ReasonOutOfRange {
		t.Fatalf("reason = %s, want out_of_range", rej[0].Reason)
	}
	port, _ := s.GetInt("app.port")
	if port != 8080 {
		t.Fatalf("port = %d, want 8080 (old config still effective)", port)
	}
	t.Log("[业务:热更新] 结果:通过, 越界更新被拒绝且旧配置保持 8080")
}

func TestStore_ConcurrentHotUpdate(t *testing.T) {
	t.Log("[业务:热更新] 用例:更新期间并发读取不得出现半新半旧的混合状态")
	s := newTestStore(t)
	const writers = 4
	const readers = 8
	const rounds = 200

	var ww sync.WaitGroup
	// 写协程: 每轮把 (name,port) 成对切换, 两个键同属一份快照
	for w := 0; w < writers; w++ {
		ww.Add(1)
		go func(w int) {
			defer ww.Done()
			for i := 0; i < rounds; i++ {
				port := 1000 + w*100 + i%10
				name := string(rune('A'+w)) + "-" + string(rune('0'+i%10))
				if rej := s.Update(Source{Name: "w", Values: map[string]any{
					"app.name": name, "app.port": port,
				}}); len(rej) != 0 {
					t.Errorf("update rejected: %v", rej)
					return
				}
			}
		}(w)
	}
	// 读协程: 从同一份快照读取 (name,port), 校验二者必然一致
	stop := make(chan struct{})
	var rw sync.WaitGroup
	for r := 0; r < readers; r++ {
		rw.Add(1)
		go func() {
			defer rw.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				snap := s.Snapshot()
				name, _ := snap.GetString("app.name")
				port, _ := snap.GetInt("app.port")
				if name == "demo" && port == 8080 {
					continue // 初始快照
				}
				// 合法快照: name 形如 "A-3", port 与 writer 编号、轮次一致
				if len(name) != 3 || name[1] != '-' {
					t.Errorf("mixed state: name=%q port=%d", name, port)
					return
				}
				writer := int(name[0] - 'A')
				round := int(name[2] - '0')
				if port != 1000+writer*100+round {
					t.Errorf("mixed state: name=%q port=%d", name, port)
					return
				}
			}
		}()
	}
	ww.Wait()
	close(stop)
	rw.Wait()
	t.Log("[业务:热更新] 结果:通过, 4 写 8 读并发下未观察到混合状态")
}

func TestStore_ExportMasksSensitive(t *testing.T) {
	t.Log("[业务:脱敏导出] 用例:敏感字段在导出中必须被掩码")
	s := newTestStore(t)
	out := s.Export()
	if out["db.password"] != mask {
		t.Fatalf("db.password = %v, want masked %q", out["db.password"], mask)
	}
	if out["app.name"] != "demo" {
		t.Fatalf("app.name = %v, want demo (non-sensitive visible)", out["app.name"])
	}
	t.Log("[业务:脱敏导出] 结果:通过, db.password 已掩码, 普通字段可见")
}
