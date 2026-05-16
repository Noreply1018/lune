# 09. CPA 生命周期 UI 与审计后续增强

状态：draft

来源：从 `spec/v0.1.6/04-cpa-credential-lifecycle.md` 移出的后续增强项。

本草案不属于当前 v0.1.6 交付范围。v0.1.6 已要求同一 CPA account key 幂等更新、删除 CPA 账号同步删除 DB row / Pool membership / 磁盘 auth file、触发 embedded CPA reload signal，并保留历史 Activity、usage 和 request log。

## 后续方向

- UI 增加重复账号检测与安全清理入口。
- 登录、reload、metadata sync、quota refresh 的安全审计日志继续细化。
- 删除和重登链路提供更完整的用户可见审计摘要。

## 需要解决的问题

- 重复账号清理入口如何避免误删仍在使用的 auth file。
- 清理入口是否只对重复 CPA account key 开放，还是也支持 orphan auth file。
- 安全审计日志是否使用独立表，还是复用 notification / request log。
- reload 和 metadata sync 的成功状态如何从 embedded CPA 中可靠观测。

## 进入正式规格前的最低验收

- fake cpa-auth 文件和 fake/mock CPA management 可构造重复账号、orphan auth file、删除、reload、metadata sync 场景。
- UI 清理入口必须展示会删除的 DB row、Pool membership 和 auth file 摘要。
- 审计日志不得包含 refresh token、access token 或完整 auth file。
- 清理后重新导入同一 account key 不与旧 auth file 冲突。
