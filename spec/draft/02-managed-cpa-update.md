# 02. Managed CPA Update

状态：draft

来源：

- previous `spec/draft.md`
- previous `spec/v0.1.6/05-lightweight-runtime.md`

## 目标

允许高级用户在不改变默认 image-pinned 行为的前提下，独立更新 embedded CPA binary。

默认用户路径仍应通过升级 Lune image 来升级 CPA。

## 可能模式

- `embedded_pinned`：默认模式。CPA 来自 Lune image，只有升级 Lune 时才变化。
- `external`：Lune 连接用户自行管理的 CPA service。
- `managed`：Lune 在用户明确触发后下载 CPA binary，并在 data directory 下运行。

## 草案方向

如果实现 `managed` mode，默认应为手动触发：

- Settings 展示当前运行 CPA version、image-pinned CPA version 和最新可用 CPA version。
- 用户点击明确的更新操作。
- Lune 只从可信 release source 下载。
- Lune 在执行前校验 checksum 或 signature。
- Lune 保留上一版 CPA binary 用于 rollback。
- 更新成功后只重启 CPA child process。
- Lune 在 system/activity logs 中记录 update attempt、version 和 failure。
- Lune 明确标记 runtime 已偏离 image-pinned CPA version。

## 非目标

- 不做静默自动 CPA binary replacement。
- v0.1.6 不实现 managed CPA binary update。

## 延后原因

Managed CPA binary update 需要：

- 可信 release metadata。
- Checksum 或 signature verification。
- Rollback。
- Update records。
- UI controls。
- 处理 container immutability。

## 当前默认路径

- Lune 升级时更新 embedded pinned CPA。
- External CPA 给高级用户提供独立升级路径。
- 不做 silent 或 automatic CPA binary replacement。

## 待决问题

- Managed CPA update 是否进入近期版本，还是保持为高级未来能力？
- Managed CPA download 应信任哪个 release metadata source 和 verification mechanism？
- health-check 失败时是否自动 rollback，还是只允许手动 rollback？
- managed mode 如何处理 container immutability 和 mounted data directory？
