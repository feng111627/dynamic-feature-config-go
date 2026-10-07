package feature

import (
	"fmt"
	"testing"
)

func TestParseFlagsStrict(t *testing.T) {
	t.Log("业务: 开关规则解析 —— 未知字段或非法比例整体拒绝，不允许部分生效")
	good := `[
		{"name":"new-checkout","enabled":true,"rule":{"percentage":25,"allow_users":["alice"]}},
		{"name":"beta-ui","enabled":false}
	]`
	flags, err := ParseFlags([]byte(good))
	if err != nil {
		t.Fatalf("parse good flags failed: %v", err)
	}
	if len(flags) != 2 {
		t.Fatalf("got %d flags, want 2", len(flags))
	}
	t.Log("  用例[合法配置解析] 通过")

	bad := []struct {
		name string
		json string
	}{
		{"未知字段", `[{"name":"a","enabled":true,"rule":{"percentage":10,"unknown_field":1}}]`},
		{"比例为负", `[{"name":"a","enabled":true,"rule":{"percentage":-5}}]`},
		{"比例超过100", `[{"name":"a","enabled":true,"rule":{"percentage":100.5}}]`},
		{"名称为空", `[{"name":"","enabled":true}]`},
		{"重名开关", `[{"name":"a","enabled":true},{"name":"a","enabled":false}]`},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			flags, err := ParseFlags([]byte(c.json))
			if err == nil {
				t.Fatalf("expected rejection, got flags=%v", flags)
			}
			if flags != nil {
				t.Fatalf("partial result leaked on rejection: %v", flags)
			}
			t.Logf("  用例[%s] 通过: 整体拒绝, err=%v", c.name, err)
		})
	}
}

func TestEvaluateConsistency(t *testing.T) {
	t.Log("业务: 灰度放量 —— 同一用户多次评估结果一致，比例边界行为正确")
	flags, err := ParseFlags([]byte(`[
		{"name":"rollout-50","enabled":true,"rule":{"percentage":50}},
		{"name":"always-off","enabled":false,"rule":{"percentage":100}},
		{"name":"zero","enabled":true,"rule":{"percentage":0}},
		{"name":"full","enabled":true,"rule":{"percentage":100}},
		{"name":"whitelist","enabled":true,"rule":{"percentage":0,"allow_users":["vip-user"]}},
		{"name":"no-rule","enabled":true}
	]`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	ev := NewEvaluator(flags)

	// 一致性：同一用户评估 100 次结果不变。
	for i := 0; i < 20; i++ {
		user := fmt.Sprintf("user-%d", i)
		first := ev.Enabled("rollout-50", user)
		for j := 0; j < 100; j++ {
			if got := ev.Enabled("rollout-50", user); got != first {
				t.Fatalf("inconsistent result for %s: %v then %v", user, first, got)
			}
		}
	}
	t.Log("  用例[20 用户 x 100 次重复评估一致] 通过")

	// 边界行为。
	cases := []struct {
		name, flag, user string
		want             bool
	}{
		{"开关关闭即使100%也不命中", "always-off", "anyone", false},
		{"0% 放量无人命中", "zero", "user-1", false},
		{"100% 放量全员命中", "full", "user-1", true},
		{"白名单用户直接命中", "whitelist", "vip-user", true},
		{"非白名单用户按0%不命中", "whitelist", "user-1", false},
		{"无规则开关启用即全员命中", "no-rule", "user-1", true},
		{"未知开关不命中", "missing-flag", "user-1", false},
	}
	for _, c := range cases {
		if got := ev.Enabled(c.flag, c.user); got != c.want {
			t.Errorf("case %q: got %v, want %v", c.name, got, c.want)
		} else {
			t.Logf("  用例[%s] 通过", c.name)
		}
	}

	// 放量比例统计：50% 规则下命中率应在合理区间。
	hits := 0
	const total = 20000
	for i := 0; i < total; i++ {
		if ev.Enabled("rollout-50", fmt.Sprintf("stat-user-%d", i)) {
			hits++
		}
	}
	ratio := float64(hits) / total * 100
	if ratio < 45 || ratio > 55 {
		t.Fatalf("rollout ratio %.2f%% out of expected band [45%%,55%%]", ratio)
	}
	t.Logf("  用例[50%% 放量统计] 通过: 实际命中率 %.2f%%", ratio)
}
