# dynamic-feature-config-go

本地的动态配置与功能开关组件：多来源配置合并为一份对外稳定的有效配置，支持不重启进程的安全热更新。

## 包结构

| 包 | 职责 |
|---|---|
| `config` | 多来源合并、Schema 校验、并发安全的热更新管理器、敏感字段脱敏 |
| `feature` | 功能开关与灰度规则的严格解析和按用户维度的一致性评估 |

## 配置来源与优先级

每个配置来源实现 `config.Source` 接口：

```go
type Source interface {
    Name() string
    Priority() int            // 数值越大优先级越高
    Load() (map[string]any, error)
}
```

`config.Merge` 按优先级**从低到高**依次加载并合并，高优先级来源的同名键覆盖低优先级的取值（例如：defaults < 本地文件 < 环境变量）。内置 `MapSource` 可直接包装内存 map，文件/环境等来源可自行实现接口。

合并结果会按 `Schema` 校验，以下情况**整体拒绝**并返回可区分的 `*ValidationError`（`Reason` 字段）：

| Reason | 含义 |
|---|---|
| `missing_key` | 必填键缺失 |
| `type_mismatch` | 值类型与声明不符 |
| `out_of_range` | 数值越出 [Min, Max] |
| `unknown_field` | 出现 Schema 未声明的键 |
| `source_load` | 来源加载失败 |

## 热更新语义

`Manager` 内部用原子快照保存当前生效配置：

- `Reload()` 重新合并并校验所有来源，**全部通过才整体替换**快照；任何一步失败都保留上一份可用配置并返回错误。
- 更新期间并发读取要么看到旧快照、要么看到新快照，**不会出现半新半旧的混合状态**（有 `-race` 测试覆盖）。
- `Snapshot()` 返回脱敏副本：Schema 中标记 `Sensitive: true` 的键（如密码、密钥）在导出与日志中显示为 `***`；`Raw()` 仅供内部使用。

## 开关规则语义

开关以 JSON 描述，`feature.ParseFlags` 严格解析：

```json
[
  {"name": "new-checkout", "enabled": true,
   "rule": {"percentage": 25, "allow_users": ["alice"]}}
]
```

- `enabled: false` 时任何用户都不命中。
- 无 `rule` 时启用即全员命中。
- `allow_users` 白名单用户直接命中。
- `percentage` 取值 `[0, 100]`，按 `(开关名, 用户ID)` 的稳定哈希（FNV-1a）分桶，**同一用户多次评估结果一致**，且放量比例调整只影响边界桶。
- 规则中出现**未知字段、非法比例、空名称或重名开关**时整体拒绝，返回错误，绝不部分生效。

## 本地验证

```bash
go build ./...        # 编译
go test ./...         # 全部单测
go test -v ./...      #  verbose：日志中可见各业务用例与结果
go test -race ./...   # 竞态检测（含并发热更新用例）
```

测试覆盖的关键边界：

- `config`：合并优先级覆盖顺序、五类拒绝原因、非法配置回退后旧配置继续生效、8 读协程 × 50 次热更新无混合状态、敏感字段脱敏与副本隔离。
- `feature`：严格解析整体拒绝（未知字段/非法比例/空名/重名）、重复评估一致性、0%/100%/白名单/未知开关边界、50% 放量统计命中率。
