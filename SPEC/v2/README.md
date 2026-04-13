# Lune v2 Spec

> v2 的主线是把 account 从"手填上游凭据"扩展为"可被路由层选中的上游执行单元"，正式支持 CPA 承载的多 provider 能力。

## 核心原则

- 保留 v1 的用户心智模型：accounts -> pools -> routes -> tokens
- 不回退到旧版 platform / account_pool 架构
- account 扩展为支持多来源（openai_compat / cpa）
- CPA 是外部镜像（eceasy/cli-proxy-api），不可修改源码
- 需要与 CPA 交互的管理能力（登录、导入）由 Lune 自己实现
- 前端新增功能与现有设计风格保持一致

## 关键决策

| 决策项 | 结论 | 理由 |
|---|---|---|
| CPA Service ID 类型 | 整数自增 | 与 v1 所有资源一致 |
| CPA 前端管理 | 侧边栏独立页面（单实例设置页） | 层次清晰，首版只支持 1 个 CPA |
| 首版 CPA 数量 | 仅 1 个，数据模型预留多个 | 简化首版实现，前端+后端均做校验 |
| Phase A CPA 账号粒度 | provider channel 维度 | CPA 内部管理多凭据并自行均衡，Lune 看到的是 provider 通道能力 |
| Phase B CPA 账号粒度 | email/account 维度 | Lune adapter 直接读取 cpa-auth 凭据文件 |
| CPA 管理能力实现者 | Lune 自己（adapter 模式） | CPA 是外部镜像不可改，Lune 实现 device code 登录和凭据扫描 |
| Login session 存储 | 内存，同一时刻仅允许 1 个 active session | 短期会话，重启自动失效 |
| Account API 路径 | 统一 /admin/api/accounts，body 区分 source_kind | 最小化路由变更 |
| runtime 块 | 响应时动态计算，不存 DB | 避免数据冗余和同步问题 |

## 实施阶段

### [Phase A: CPA 作为 Provider](./phase-a.md)

**前置条件：无（基于 CPA 现有推理接口）**

利用 CPA 已有的 `/api/provider/{provider}/v1/` 路由能力，让 Lune 原生支持 CPA provider channel。核心变更：

- account 增加 source_kind 字段
- 新增 cpa_services 表（首版限 1 条）
- CPA 账号 = CPA Service + provider（provider-backed logical account）
- 前端新增 CPA Service 单实例设置页
- Accounts 页按来源创建，CPA 类型明确标注为 provider channel
- 运行时透明转发到 CPA 的 provider 接口
- 部署层面定义 cpa-auth 共享目录

### [Phase B: CPA 管理 Adapter](./phase-b.md)

**前置条件：Phase A 完成 + cpa-auth 共享卷就绪**

CPA 是外部镜像不可改。所有管理能力由 Lune 自己实现（Lune CPA Management Adapter）：

- Lune 直接实现 OpenAI OAuth Device Code Flow
- Lune 将凭据写入 cpa-auth/ 共享目录，CPA 热加载识别
- Lune 扫描 cpa-auth/ 目录导入已有账号
- 账号粒度从 provider 扩展到单个 email/account
- 到期预警、元数据同步

### [Phase C: 体验增强与分析](./phase-c.md)

**前置条件：无（独立于 CPA 改造）**

- OpenAI-compatible 侧 provider 模板
- 一键测试连接
- 环境变量代码片段增强
- 内置 Playground
- 成本估算与延迟追踪

## CPA 现状（eceasy/cli-proxy-api）

v2 设计基于 CPA 的实际接口能力：

| 接口 | 状态 | 说明 |
|---|---|---|
| /healthz | 可用 | 健康检查 |
| /v1/models | 可用 | 全部可用模型列表 |
| /v1/chat/completions | 可用 | OpenAI 兼容推理 |
| /v1/completions | 可用 | Text Completions |
| /v1/responses | 可用 | OpenAI Responses API |
| /api/provider/{provider}/v1/... | 可用 | **按 provider 路由（Phase A 核心依赖）** |
| /management.html | 可用 | 管理面板（HTML，非 REST API） |
| /admin/accounts | 不存在 | CPA 无管理 API，Phase B 由 Lune adapter 取代 |
| /admin/login/device/start | 不存在 | Lune adapter 直接对接 OpenAI OAuth |

已验证可用的 CPA provider：

- codex - ChatGPT Plus/Pro 账号
- claude - Claude
- gemini / gemini-cli - Gemini
- vertex - Vertex AI
- aistudio - AI Studio
- openai - OpenAI 原生
- qwen - 通义千问
- kimi - Kimi
- iflow - iFlow (GLM)
- antigravity - Antigravity

## 部署拓扑

```
Host / Docker Compose
 +-- lune container (port 7788)
 |   +-- /app/data        -> lune-data volume (SQLite)
 |   +-- /app/cpa-auth    -> cpa-auth volume (shared, r/w)
 |
 +-- lune-upstream-node container (CPA, port 8317)
 |   +-- /app/cpa-auth    -> cpa-auth volume (shared, r/w)
 |   +-- /app/cpa-config   -> CPA config
 |
 +-- cpa-auth volume (named, shared between lune & CPA)
     +-- codex-user@mail.com-plus.json
     +-- codex-user2@mail.com-pro.json
     +-- ...
```

关键约束：
- Lune 和 CPA 必须挂载同一个 cpa-auth 目录
- Phase A 只读（扫描元信息），Phase B 读写（登录写入新凭据）
- CPA 热加载 cpa-auth 目录变更，无需重启
- 配置项：LUNE_CPA_AUTH_DIR 环境变量或 lune.yaml 中的 cpa_auth_dir
