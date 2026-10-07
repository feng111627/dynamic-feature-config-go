# dynamic-feature-config-go

本地的动态配置与功能开关组件：多来源配置合并成一份对外稳定的有效配置，并支持不重启进程的安全热更新。

## 包结构

| 包 | 职责 |
| --- | --- |
| `config` | 多来源配置合并、Schema 校验、原子热更新、敏感字段脱敏 |
| `feature` | 功能开关与灰度规则、确定性用户放量、原子热更新 |

## 配置来源与优先级

`config.Merge(schema, sources...)` 按传入顺序合并来源，**后加载的来源覆盖先前取值**，未覆盖的键保留先前来源的值：

```go
schema := config.Schema{
    "app.name":    {Type: config.TypeString, Required: true},
    "app.port":    {Type: config.TypeInt, Required: true, Min: f(1), Max: f(65535)},
    "db.password": {Type: config.TypeString, Sensitive: true},
}
store, rej := config.NewStore(schema,
    config.Source{Name: "file", Values: fileValues},   // 优先级低
    config.Source{Name: "env", Values: envValues},     // 优先级高, 覆盖 file
)
```

### 校验与拒绝语义

合并时依据 Schema 严格校验，**任何一项非法即整体拒绝，绝不部分套用，也不静默回填默认值**。拒绝原因可区分：

| 拒绝原因 | 触发条件 |
| --- | --- |
| `missing_key` | 合并所有来源后仍缺少 `Required` 键 |
| `type_mismatch` | 取值类型与 Schema 声明不符 |
| `out_of_range` | 数值超出 `Min`/`Max` 约束 |
| `unknown_key` | 来源中出现 Schema 未定义的键 |

热更新时调用 `store.Update(sources...)`：校验通过则**原子整体切换**；校验失败返回全部 `Rejection`，**上一份可用配置继续生效**。

### 并发安全与一致性读取

- 快照通过 `atomic.Pointer` 整体切换，更新期间并发读取不会看到半新半旧的混合状态。
- 需要一致地读取多个键时，使用同一份快照：

```go
snap := store.Snapshot()
name, _ := snap.GetString("app.name")
port, _ := snap.GetInt("app.port")
```

### 敏感字段脱敏

`store.Export()` 导出的快照中，`Sensitive: true` 的键被替换为 `***`，可安全用于日志与对外接口。

## 功能开关与灰度规则

开关以 JSON 描述，通过 `feature.NewStore` / `store.Update` 加载：

```json
[
  {
    "name": "new-ui",
    "enabled": true,
    "rule": { "percentage": 25, "allowUsers": ["alice"] }
  }
]
```

评估语义（`store.Enabled(flagName, userID)`）：

1. 开关不存在或 `enabled=false` → 关闭；
2. 用户在 `allowUsers` 白名单中 → 直接命中；
3. 否则按 `percentage` 放量：对 `(flagName, userID)` 做 FNV-1a 哈希取模 100 分桶，桶值小于 `percentage` 才命中。**哈希对同一用户稳定，因此同一用户多次评估结果必然一致**；
4. `percentage=0` 全关（白名单除外），`percentage=100` 全开。

### 严格校验

规则解析是严格的，任一非法即**整体拒绝**，不能只生效一部分：

- 未知字段（含 `rule` 内部）→ 拒绝（`DisallowUnknownFields`）；
- `percentage` 不在 `[0,100]` → 拒绝；
- 开关名为空或重复 → 拒绝。

`Update` 校验失败时保留旧开关集合继续生效。

## 本地验证

```bash
# 构建
go build ./...

# 运行全部单测（-race 检测并发问题, -v 查看各业务用例与结果）
go test ./... -race -v
```

测试日志按 `[业务:xxx] 用例:... / 结果:...` 输出，覆盖：

- **配置合并**：后加载来源覆盖优先级；
- **配置校验**：类型不匹配 / 越界 / 未知键 / 缺必填键的拒绝原因；
- **热更新**：非法更新回退旧配置、4 写 8 读并发下无混合状态；
- **脱敏导出**：敏感字段掩码；
- **灰度放量**：同一用户多次评估一致、0%/100%/白名单边界语义；
- **开关校验**：未知字段、非法比例、空名、重名的整体拒绝；
- **开关热更新**：非法更新回退、并发评估与更新无竞争。
