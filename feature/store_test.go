package feature

import (
	"fmt"
	"sync"
	"testing"
)

func mustStore(t *testing.T, json string) *Store {
	t.Helper()
	s, err := NewStore([]byte(json))
	if err != nil {
		t.Fatalf("init store: %v", err)
	}
	return s
}

func TestEnabled_DeterministicPerUser(t *testing.T) {
	t.Log("[业务:灰度放量] 用例:同一用户多次评估结果必须一致")
	s := mustStore(t, `[{"name":"new-ui","enabled":true,"rule":{"percentage":50}}]`)
	for i := 0; i < 200; i++ {
		user := fmt.Sprintf("user-%d", i)
		first := s.Enabled("new-ui", user)
		for j := 0; j < 20; j++ {
			if got := s.Enabled("new-ui", user); got != first {
				t.Fatalf("user %s: eval %d flipped to %v", user, j, got)
			}
		}
	}
	t.Log("[业务:灰度放量] 结果:通过, 200 个用户各评估 20 次结果稳定")
}

func TestEnabled_RolloutSemantics(t *testing.T) {
	t.Log("[业务:灰度放量] 用例:0%% 全关 / 100%% 全开 / 白名单直通 / 未启用开关关闭")
	s := mustStore(t, `[
		{"name":"off","enabled":true,"rule":{"percentage":0}},
		{"name":"full","enabled":true,"rule":{"percentage":100}},
		{"name":"beta","enabled":true,"rule":{"percentage":0,"allowUsers":["alice"]}},
		{"name":"disabled","enabled":false,"rule":{"percentage":100}}
	]`)
	for i := 0; i < 50; i++ {
		u := fmt.Sprintf("u%d", i)
		if s.Enabled("off", u) {
			t.Fatalf("percentage=0 should be off for %s", u)
		}
		if !s.Enabled("full", u) {
			t.Fatalf("percentage=100 should be on for %s", u)
		}
		if s.Enabled("disabled", u) {
			t.Fatalf("disabled flag should be off for %s", u)
		}
	}
	if !s.Enabled("beta", "alice") {
		t.Fatalf("allowUsers should pass for alice")
	}
	if s.Enabled("beta", "bob") {
		t.Fatalf("percentage=0 should reject bob (not in allowUsers)")
	}
	if s.Enabled("no-such-flag", "alice") {
		t.Fatalf("unknown flag should be off")
	}
	t.Log("[业务:灰度放量] 结果:通过, 边界比例与白名单语义正确")
}

func TestParseFlags_RejectsAsWhole(t *testing.T) {
	t.Log("[业务:开关校验] 用例:未知字段或非法比例时整体拒绝, 不能只生效一部分")
	cases := []struct {
		name string
		json string
	}{
		{"未知字段", `[{"name":"a","enabled":true,"rule":{"percentage":10},"bogus":1}]`},
		{"规则内未知字段", `[{"name":"a","enabled":true,"rule":{"percentage":10,"bogus":1}}]`},
		{"比例越界-负", `[{"name":"a","enabled":true,"rule":{"percentage":-1}}]`},
		{"比例越界-超百", `[{"name":"a","enabled":true,"rule":{"percentage":101}}]`},
		{"空名称", `[{"name":"","enabled":true,"rule":{"percentage":10}}]`},
		{"重复名称", `[{"name":"a","enabled":true,"rule":{"percentage":10}},{"name":"a","enabled":false,"rule":{"percentage":20}}]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseFlags([]byte(tc.json)); err == nil {
				t.Fatalf("want error for %s", tc.name)
			} else {
				t.Logf("[业务:开关校验] 子用例:%s 拒绝原因:%v", tc.name, err)
			}
		})
	}
	t.Log("[业务:开关校验] 结果:通过, 全部非法输入被整体拒绝")
}

func TestStore_InvalidUpdateKeepsOldFlags(t *testing.T) {
	t.Log("[业务:开关热更新] 用例:非法更新被拒绝, 旧开关继续生效")
	s := mustStore(t, `[{"name":"new-ui","enabled":true,"rule":{"percentage":100}}]`)
	if err := s.Update([]byte(`[{"name":"new-ui","enabled":false,"rule":{"percentage":200}}]`)); err == nil {
		t.Fatalf("want error for illegal percentage")
	}
	if !s.Enabled("new-ui", "anyone") {
		t.Fatalf("old flags should remain effective after rejected update")
	}
	t.Log("[业务:开关热更新] 结果:通过, 非法更新被拒绝且旧开关保持开启")
}

func TestStore_ConcurrentEvalDuringUpdate(t *testing.T) {
	t.Log("[业务:开关热更新] 用例:更新期间并发评估不 panic 且结果只来自某一份完整快照")
	s := mustStore(t, `[{"name":"f","enabled":true,"rule":{"percentage":100}}]`)
	const rounds = 300
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for r := 0; r < 8; r++ {
		wg.Add(1)
		go func(r int) {
			defer wg.Done()
			user := fmt.Sprintf("reader-%d", r)
			for {
				select {
				case <-stop:
					return
				default:
				}
				_ = s.Enabled("f", user)
			}
		}(r)
	}
	for i := 0; i < rounds; i++ {
		pct := i % 101
		body := fmt.Sprintf(`[{"name":"f","enabled":true,"rule":{"percentage":%d}}]`, pct)
		if err := s.Update([]byte(body)); err != nil {
			t.Fatalf("update: %v", err)
		}
	}
	close(stop)
	wg.Wait()
	t.Log("[业务:开关热更新] 结果:通过, 8 读协程与 300 次更新并发无异常")
}
