package config

import "testing"

func f64(v float64) *float64 { return &v }

func testSchema() Schema {
	return Schema{
		"app.name":    {Type: TypeString, Required: true},
		"app.port":    {Type: TypeInt, Required: true, Min: f64(1), Max: f64(65535)},
		"app.debug":   {Type: TypeBool},
		"app.ratio":   {Type: TypeFloat, Min: f64(0), Max: f64(1)},
		"db.password": {Type: TypeString, Sensitive: true},
	}
}

func TestMerge_LaterSourceOverrides(t *testing.T) {
	t.Log("[业务:配置合并] 用例:后加载来源覆盖先前取值")
	base := Source{Name: "file", Values: map[string]any{
		"app.name": "demo", "app.port": 8080, "app.debug": false,
	}}
	overlay := Source{Name: "env", Values: map[string]any{
		"app.port": 9090, "app.debug": true,
	}}
	cfg, rej := Merge(testSchema(), base, overlay)
	if len(rej) != 0 {
		t.Fatalf("unexpected rejections: %v", rej)
	}
	if got := cfg.values["app.port"]; got != 9090 {
		t.Fatalf("app.port = %v, want 9090 (env overrides file)", got)
	}
	if got := cfg.values["app.debug"]; got != true {
		t.Fatalf("app.debug = %v, want true", got)
	}
	if got := cfg.values["app.name"]; got != "demo" {
		t.Fatalf("app.name = %v, want demo (kept from base)", got)
	}
	t.Log("[业务:配置合并] 结果:通过, env 覆盖 file, 未覆盖键保留")
}

func TestMerge_RejectReasons(t *testing.T) {
	cases := []struct {
		name   string
		source Source
		want   RejectReason
	}{
		{"类型不匹配", Source{Name: "bad", Values: map[string]any{"app.port": "8080"}}, ReasonTypeMismatch},
		{"取值越界", Source{Name: "bad", Values: map[string]any{"app.port": 70000}}, ReasonOutOfRange},
		{"未知键", Source{Name: "bad", Values: map[string]any{"app.unknown": 1}}, ReasonUnknownKey},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("[业务:配置校验] 用例:%s 应整体拒绝", tc.name)
			base := Source{Name: "file", Values: map[string]any{"app.name": "demo", "app.port": 8080}}
			cfg, rej := Merge(testSchema(), base, tc.source)
			if cfg != nil {
				t.Fatalf("config should be nil on rejection")
			}
			found := false
			for _, r := range rej {
				if r.Reason == tc.want {
					found = true
				}
			}
			if !found {
				t.Fatalf("want reason %q in %v", tc.want, rej)
			}
			t.Logf("[业务:配置校验] 结果:通过, 拒绝原因=%s", tc.want)
		})
	}
}

func TestMerge_MissingRequiredKey(t *testing.T) {
	t.Log("[业务:配置校验] 用例:缺少必填键不得静默套用默认值")
	cfg, rej := Merge(testSchema(), Source{Name: "file", Values: map[string]any{"app.name": "demo"}})
	if cfg != nil {
		t.Fatalf("config should be nil when required key missing")
	}
	found := false
	for _, r := range rej {
		if r.Reason == ReasonMissingKey && r.Key == "app.port" {
			found = true
		}
	}
	if !found {
		t.Fatalf("want missing_key for app.port, got %v", rej)
	}
	t.Log("[业务:配置校验] 结果:通过, 缺少 app.port 被拒绝且无默认值兜底")
}
