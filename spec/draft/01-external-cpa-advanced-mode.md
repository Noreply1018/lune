# 01. 外部 CPA Advanced Mode

状态：draft

来源：

- previous `spec/draft.md`
- previous `spec/v0.1.6/05-lightweight-runtime.md`

## 目标

保持 all-in-one embedded CPA 作为默认体验，同时提供清晰的 opt-in 外部 CPA 路径。

适用场景：

- 用户需要独立升级 CPA。
- 用户已有外部 CPA 部署。
- 用户需要自定义 CPA 配置。
- 用户需要独立调试 CPA。

## 背景

Embedded CPA 给 Lune 更清晰的默认产品边界，但一部分高级用户可能需要外部 CPA：

- 独立于 Lune release 更快升级 CPA。
- 使用自定义 CPA 配置。
- 复用已有 CPA 部署。
- 独立调试或运维 CPA。

## 草案方向

- Default 仍为 embedded CPA。
- External CPA 必须 opt-in。
- External CPA 使用明确的 Lune environment variables。
- 文档必须标注为 advanced。
- quick start 不应优先展示 external CPA。
- Docker Compose 和 `.env.example` 应提供一致路径，不让用户从内部变量推断。
- Settings 应区分 `embedded` 与 `external` runtime mode。

## 建议配置形态

已有变量可作为基础：

```env
LUNE_EMBEDDED_CPA=0
LUNE_CPA_BASE_URL=
LUNE_CPA_API_KEY=
LUNE_CPA_MANAGEMENT_KEY=
```

文档必须明确回答：

- 如何禁用 embedded CPA。
- Lune 连接哪个 CPA base URL。
- provider requests 使用哪个 API key。
- CPA management operations 使用哪个 management key。

## 最低验收

- Docker 默认 embedded CPA。
- External CPA 不需要 patch internal files 即可配置。
- Settings 连接外部 CPA 时显示 `external`。
- 测试覆盖 embedded vs external runtime-mode detection/config parsing。
- 如果多个 Lune CPA account 共用一个 external CPA runtime，而 runtime 不支持 per-request pinning，文档必须说明账号统计和额度归因不可信。

## 非目标

- 不恢复复杂旧双容器 quick start 作为默认路径。
- 不增加 migration wizard。
- 不做 CPA on-demand suspend/resume。
- 不做 managed CPA binary updates。

## 后续扩展

- 推动 CLIProxyAPI upstream 正式支持外部 per-request auth pinning，减少 Lune 本地 patch 维护成本。
- 定义非 embedded CPA 如何声明 pinning 能力，以及能力不足时的用户可见诊断。

## 待决问题

- 生产 Compose 文档中使用哪个环境变量禁用 embedded CPA？
- 配置 external CPA 时，Lune 是否仍自动创建或更新 CPA service row？
- External CPA 是否要求用户同时提供 API key 和 management key？
- 旧双容器 Compose 文档应保留多少？
- 测试是否覆盖 embedded CPA 与 external CPA 之间的切换？
