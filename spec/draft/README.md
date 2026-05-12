# Draft 规格草案

状态：draft

本目录收纳已经从历史版本规格中移出的设计草案。这些内容不属于当前 v0.1.6 交付范围，后续版本需要重新评估、拆分和验收后再进入具体 release spec。

## 文档结构

- `01-external-cpa-advanced-mode.md`：外部 CPA advanced mode，适合用户自行运行或独立升级 CPA。
- `02-managed-cpa-update.md`：由 Lune 管理 CPA binary 更新的未来方案。
- `03-large-non-stream-response-streaming.md`：大型非流式响应的内存保护与流式转发。
- `04-lightweight-vps-runtime.md`：轻量 VPS、native systemd、low-resource profile、SQLite/日志/health check/内存控制。

## 迁移说明

- 原 `spec/draft.md` 的草案已拆分到本目录。
- 原 `spec/v0.1.6/05-lightweight-runtime.md` 已按主题拆入本目录，不再作为 v0.1.6 范围。
- draft 内容可以保留 open questions 和未决取舍；进入正式版本规格前必须重新收束目标、验收标准和实现边界。
