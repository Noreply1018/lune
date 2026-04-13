# Phase A: CPA 作为 Provider

> 基于 CPA 现有推理接口，无需 CPA 侧改动。Lune 将 CPA 的每个 provider 视为一个 provider-backed logical account。

## 1. 背景

### 1.1 现状问题

v1 的 Accounts 默认假设上游都是 OpenAI 兼容服务（base_url + api_key）。这在直连 OpenAI / DeepSeek / Groq 等服务时工作良好，但不适合 CPA 承载的 GPT/Codex 账号链路：

```
v1:  Client -> Lune -> OpenAI (base_url + api_key)
CPA: Client -> Lune -> CPA /api/provider/{provider}/v1 -> OpenAI/ChatGPT
```

v1 下用户只能把 CPA 伪装成一个普通 OpenAI 兼容服务，丢失了 CPA 多 provider 路由、服务健康感知等能力。

### 1.2 Phase A 目标

让 Lune 原生理解两类账号来源：

1. **openai_compat** - 继续使用 base_url + api_key 直连上游
2. **cpa** - 通过 CPA Service 的 provider 路由转发

Phase A 的 CPA 账号粒度是 **provider channel 维度**（如 codex、claude、gemini），不是单个 email/account。

语义说明：

- Phase A 的一个 CPA account 实际上是 **provider-backed logical account**
- 它代表的是"CPA 的 codex 通道能力"，不是"某个 GPT 邮箱账号"
- CPA 内部管理多个凭据并自行做负载均衡
- usage/health 统计反映的是 **provider 聚合视角**，不是单个 email 视角
- 数据模型预留 cpa_account_key 字段，Phase B 可扩展到单账号粒度

### 1.3 不做什么

- 不恢复旧版 platform / account_pool 抽象
- 不把 CPA 并入 Lune
- 不实现设备码登录（Phase B）
- 不实现单账号维度管理（Phase B）
- 不改 pool 和 route 的用户心智模型

---

## 2. 数据模型

### 2.1 数据库 Schema 变更

#### 新增表：cpa_services

```sql
CREATE TABLE IF NOT EXISTS cpa_services (
    id              INTEGER PRIMARY KEY,
    label           TEXT NOT NULL,
    base_url        TEXT NOT NULL,
    api_key         TEXT NOT NULL DEFAULT '',
    enabled         INTEGER NOT NULL DEFAULT 1,
    status          TEXT NOT NULL DEFAULT 'unknown',
    last_checked_at TEXT,
    last_error      TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);
```

字段说明：

| 字段 | 说明 |
|---|---|
| base_url | CPA 服务根地址，如 http://127.0.0.1:8317 |
| api_key | CPA 的 Bearer token，如 sk-cpa-default |
| status | unknown / healthy / error，由健康检查更新 |

首版约束：创建时校验 cpa_services 表行数，已存在 1 条则拒绝（409）。

#### 扩展表：accounts

```sql
-- 新增字段
ALTER TABLE accounts ADD COLUMN source_kind     TEXT NOT NULL DEFAULT 'openai_compat';
ALTER TABLE accounts ADD COLUMN provider         TEXT NOT NULL DEFAULT '';
ALTER TABLE accounts ADD COLUMN cpa_service_id   INTEGER REFERENCES cpa_services(id);
ALTER TABLE accounts ADD COLUMN cpa_provider     TEXT NOT NULL DEFAULT '';
ALTER TABLE accounts ADD COLUMN cpa_account_key  TEXT NOT NULL DEFAULT '';

-- CPA 账号唯一约束：同一 service + provider 不重复
CREATE UNIQUE INDEX IF NOT EXISTS idx_accounts_cpa_unique
    ON accounts(cpa_service_id, cpa_provider)
    WHERE source_kind = 'cpa' AND cpa_provider != '';
```

字段说明：

| 字段 | 说明 |
|---|---|
| source_kind | openai_compat 或 cpa |
| provider | OpenAI-compatible 侧的模板标识，如 openai、deepseek，仅标注用 |
| cpa_service_id | 关联的 CPA Service ID，仅 source_kind=cpa 时有值 |
| cpa_provider | CPA 的 provider 标识，如 codex、claude、gemini |
| cpa_account_key | 预留字段，Phase A 留空，Phase B 填入 email/account 级别标识 |

#### 扩展表：request_logs

```sql
ALTER TABLE request_logs ADD COLUMN source_kind TEXT NOT NULL DEFAULT 'openai_compat';
```

#### 对 base_url 和 api_key 的语义变化

对于 source_kind=cpa 的账号：

- base_url：留空。运行时动态派生
- api_key：留空。运行时使用 cpa_service.api_key

对于 source_kind=openai_compat 的账号：

- base_url / api_key：保持 v1 语义

### 2.2 v1 -> v2 迁移

迁移脚本追加到 store.go 的 migrations 数组（schema_version 从 1 升至 2）：

```sql
-- Migration v1 -> v2

-- 1. 创建 cpa_services 表
CREATE TABLE IF NOT EXISTS cpa_services (
    id              INTEGER PRIMARY KEY,
    label           TEXT NOT NULL,
    base_url        TEXT NOT NULL,
    api_key         TEXT NOT NULL DEFAULT '',
    enabled         INTEGER NOT NULL DEFAULT 1,
    status          TEXT NOT NULL DEFAULT 'unknown',
    last_checked_at TEXT,
    last_error      TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

-- 2. 扩展 accounts 表
ALTER TABLE accounts ADD COLUMN source_kind     TEXT NOT NULL DEFAULT 'openai_compat';
ALTER TABLE accounts ADD COLUMN provider         TEXT NOT NULL DEFAULT '';
ALTER TABLE accounts ADD COLUMN cpa_service_id   INTEGER REFERENCES cpa_services(id);
ALTER TABLE accounts ADD COLUMN cpa_provider     TEXT NOT NULL DEFAULT '';
ALTER TABLE accounts ADD COLUMN cpa_account_key  TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_accounts_cpa_unique
    ON accounts(cpa_service_id, cpa_provider)
    WHERE source_kind = 'cpa' AND cpa_provider != '';

-- 3. 扩展 request_logs 表
ALTER TABLE request_logs ADD COLUMN source_kind TEXT NOT NULL DEFAULT 'openai_compat';
```

迁移保证：

- 所有现有 accounts 自动获得 source_kind='openai_compat'（DEFAULT 值）
- 所有现有 request_logs 自动获得 source_kind='openai_compat'
- 无破坏性变更，v1 功能完全保留

### 2.3 Pool 与 Route

**结构不变。** 补充语义说明：

- pool 可混合包含 openai_compat 与 cpa 账号
- 路由层不感知 account 来源类型
- 来源差异只由运行时转发分支处理

### 2.4 Go 模型扩展

#### CpaService

```go
type CpaService struct {
    ID            int64   `json:"id"`
    Label         string  `json:"label"`
    BaseURL       string  `json:"base_url"`
    APIKey        string  `json:"api_key,omitempty"`
    APIKeySet     bool    `json:"api_key_set"`
    APIKeyMasked  string  `json:"api_key_masked"`
    Enabled       bool    `json:"enabled"`
    Status        string  `json:"status"`
    LastCheckedAt *string `json:"last_checked_at"`
    LastError     string  `json:"last_error"`
    CreatedAt     string  `json:"created_at"`
    UpdatedAt     string  `json:"updated_at"`
}
```

#### Account 扩展

```go
type Account struct {
    // v1 保留字段
    ID             int64    `json:"id"`
    Label          string   `json:"label"`
    BaseURL        string   `json:"base_url"`
    APIKey         string   `json:"api_key,omitempty"`
    Enabled        bool     `json:"enabled"`
    Status         string   `json:"status"`
    QuotaTotal     float64  `json:"quota_total"`
    QuotaUsed      float64  `json:"quota_used"`
    QuotaUnit      string   `json:"quota_unit"`
    Notes          string   `json:"notes"`
    ModelAllowlist []string `json:"model_allowlist"`
    LastCheckedAt  *string  `json:"last_checked_at"`
    LastError      string   `json:"last_error"`
    CreatedAt      string   `json:"created_at"`
    UpdatedAt      string   `json:"updated_at"`

    // v2 新增
    SourceKind    string `json:"source_kind"`
    Provider      string `json:"provider"`
    CpaServiceID  *int64 `json:"cpa_service_id,omitempty"`
    CpaProvider   string `json:"cpa_provider,omitempty"`
    CpaAccountKey string `json:"cpa_account_key,omitempty"`

    // 响应时动态计算
    Runtime *AccountRuntime `json:"runtime,omitempty"`
}

type AccountRuntime struct {
    BaseURL  string `json:"base_url"`
    AuthMode string `json:"auth_mode"`
}
```

Runtime 派生规则：

- openai_compat: Runtime.BaseURL = Account.BaseURL
- cpa: Runtime.BaseURL = CpaService.BaseURL + "/api/provider/" + Account.CpaProvider + "/v1"
- 始终: Runtime.AuthMode = "bearer"

Runtime 不存入数据库，在 API 响应序列化时动态填充。

---

## 3. Admin API

### 3.1 CPA Service 端点

| Method | Path | 说明 |
|---|---|---|
| GET | /admin/api/cpa/service | 获取 CPA Service 配置 |
| PUT | /admin/api/cpa/service | 创建或更新 CPA Service |
| DELETE | /admin/api/cpa/service | 删除 CPA Service（有关联账号时拒绝 409） |
| POST | /admin/api/cpa/service/test | 测试连通性 |
| POST | /admin/api/cpa/service/enable | 启用 |
| POST | /admin/api/cpa/service/disable | 停用 |

注意：首版使用**单数** /cpa/service（不是复数），因为只支持一个。数据库表仍是复数 cpa_services，为未来多实例预留。

#### 创建/更新请求

```json
{
  "label": "Local CPA",
  "base_url": "http://127.0.0.1:8317",
  "api_key": "sk-cpa-default",
  "enabled": true
}
```

#### 获取响应

```json
{
  "data": {
    "id": 1,
    "label": "Local CPA",
    "base_url": "http://127.0.0.1:8317",
    "api_key_set": true,
    "api_key_masked": "sk-...ault",
    "enabled": true,
    "status": "healthy",
    "last_checked_at": "2026-04-13T12:00:00Z",
    "last_error": "",
    "created_at": "2026-04-13T10:00:00Z",
    "updated_at": "2026-04-13T10:00:00Z"
  }
}
```

无 CPA Service 时 GET 返回 `{"data": null}`。

#### 测试连通性响应

```json
{
  "data": {
    "reachable": true,
    "latency_ms": 12,
    "providers": ["codex", "claude", "gemini", "openai", "qwen", "kimi"],
    "error": ""
  }
}
```

测试逻辑：

1. 请求 {base_url}/healthz 验证服务可达
2. 请求 {base_url}/v1/models 获取模型列表，提取可用 provider
3. 返回延迟和 provider 列表

#### 删除约束

有关联的 cpa 账号时返回 409 Conflict：

```json
{
  "error": {
    "code": "has_dependent_accounts",
    "message": "Cannot delete CPA service: 3 accounts are linked to it. Remove them first."
  }
}
```

### 3.2 Account 端点

保持 v1 路径结构，通过 body 中的 source_kind 区分行为。

| Method | Path | 说明 | 变更 |
|---|---|---|---|
| GET | /admin/api/accounts | 列出账号 | 响应新增 source_kind、runtime 等字段 |
| POST | /admin/api/accounts | 创建账号 | body 按 source_kind 区分 |
| PUT | /admin/api/accounts/{id} | 更新账号 | 按 source_kind 校验可编辑字段 |
| DELETE | /admin/api/accounts/{id} | 删除账号 | 不变 |
| POST | /admin/api/accounts/{id}/enable | 启用 | 不变 |
| POST | /admin/api/accounts/{id}/disable | 停用 | 不变 |

#### 创建 OpenAI-Compatible 账号

```json
{
  "source_kind": "openai_compat",
  "label": "OpenAI Main",
  "provider": "openai",
  "base_url": "https://api.openai.com/v1",
  "api_key": "sk-...",
  "enabled": true,
  "notes": "",
  "model_allowlist": []
}
```

#### 创建 CPA Provider Channel

```json
{
  "source_kind": "cpa",
  "label": "CPA Codex",
  "cpa_service_id": 1,
  "cpa_provider": "codex",
  "enabled": true,
  "notes": "Codex models via local CPA",
  "model_allowlist": ["gpt-4o", "gpt-4.1", "o3-mini"]
}
```

后端校验规则：

- source_kind=openai_compat 时必填 base_url、api_key
- source_kind=cpa 时必填 cpa_service_id、cpa_provider，忽略 base_url、api_key
- source_kind=cpa 时检查 cpa_service_id + cpa_provider 唯一约束

#### 更新规则

| 字段 | openai_compat 可编辑 | cpa 可编辑 |
|---|---|---|
| label | 是 | 是 |
| base_url | 是 | 否（派生自 CPA Service） |
| api_key | 是 | 否（使用 CPA Service 的 key） |
| provider | 是 | 否 |
| cpa_provider | 否 | 否（创建后不可改） |
| cpa_service_id | 否 | 否（创建后不可改） |
| enabled | 是 | 是 |
| notes | 是 | 是 |
| model_allowlist | 是 | 是 |
| quota_* | 是 | 否（CPA 账号不适用） |

#### 列表响应

```json
{
  "data": [
    {
      "id": 1,
      "label": "OpenAI Main",
      "source_kind": "openai_compat",
      "provider": "openai",
      "base_url": "https://api.openai.com/v1",
      "api_key_set": true,
      "api_key_masked": "sk-...abcd",
      "enabled": true,
      "status": "healthy",
      "runtime": {
        "base_url": "https://api.openai.com/v1",
        "auth_mode": "bearer"
      },
      "cpa_service_id": null,
      "cpa_provider": ""
    },
    {
      "id": 2,
      "label": "CPA Codex",
      "source_kind": "cpa",
      "provider": "",
      "base_url": "",
      "api_key_set": false,
      "api_key_masked": "",
      "enabled": true,
      "status": "healthy",
      "runtime": {
        "base_url": "http://127.0.0.1:8317/api/provider/codex/v1",
        "auth_mode": "bearer"
      },
      "cpa_service_id": 1,
      "cpa_provider": "codex"
    }
  ],
  "total": 2
}
```

### 3.3 Usage 端点

GET /admin/api/usage 新增可选过滤参数：

| 参数 | 说明 |
|---|---|
| source_kind | 按来源类型过滤：openai_compat / cpa |

### 3.4 Overview 端点

GET /admin/api/overview 响应扩展：

```json
{
  "data": {
    "...existing v1 fields...": "",
    "cpa_status": {
      "connected": true,
      "label": "Local CPA",
      "status": "healthy",
      "accounts_total": 2,
      "accounts_healthy": 2,
      "accounts_error": 0,
      "last_checked_at": "2026-04-13T12:00:00Z"
    },
    "accounts_by_source": {
      "openai_compat": 3,
      "cpa": 2
    }
  }
}
```

首版只有一个 CPA Service，所以 cpa_status 是单一对象（非数组）。无 CPA Service 时 cpa_status 为 null。

### 3.5 Export 端点

GET /admin/api/export 响应扩展，增加 cpa_services 数组。

---

## 4. 前端交互设计

### 4.1 侧边栏导航变更

在 Shell 组件的 Configure 分组中新增入口：

```
Observe
  +-- Overview       /admin
  +-- Usage          /admin/usage
Configure
  +-- Accounts       /admin/accounts
  +-- Pools          /admin/pools
  +-- Routes         /admin/routes
  +-- Tokens         /admin/tokens
  +-- CPA Service    /admin/cpa-service    <-- 新增（单数）
```

图标建议使用 Lucide 的 Server 或 Cloud 图标。

### 4.2 CPA Service 页面（单实例设置页）

路径：/admin/cpa-service

这不是列表 CRUD 页面，而是**单实例设置页**（类似 Settings），因为首版只支持 1 个 CPA Service。

#### 未配置状态

```
CPA Service
Connect to a CPA (cli-proxy-api) instance for GPT/Codex account access.

  +--------------------------------------------+
  |  No CPA service configured.                |
  |                                            |
  |  [Configure CPA Service]                   |
  +--------------------------------------------+
```

#### 已配置状态

```
CPA Service
Manage your CPA (cli-proxy-api) connection.

  +--------------------------------------------+
  |  Local CPA                    [healthy]    |
  |                                            |
  |  Base URL    http://127.0.0.1:8317         |
  |  API Key     sk-...ault                    |
  |  Status      healthy                       |
  |  Last Check  2 min ago                     |
  |                                            |
  |  [Test Connection]  [Edit]  [Disable]      |
  +--------------------------------------------+

  Linked Accounts
  2 provider channels using this service.

  +------+------------+-----------+---------+
  | ID   | Label      | Provider  | Status  |
  +------+------------+-----------+---------+
  | 2    | CPA Codex  | codex     | healthy |
  | 3    | CPA Claude | claude    | healthy |
  +------+------------+-----------+---------+
```

#### 编辑 Dialog

```
Label          [________________]
Base URL       [________________]   (placeholder: http://127.0.0.1:8317)
API Key        [________________]   (placeholder: sk-cpa-default)
Enabled        [toggle]
```

#### 测试连接

点击 Test Connection 后：

- 成功：toast "Connected (12ms) - 6 providers available"
- 失败：toast 错误详情

#### 删除

底部或菜单中提供 Remove CPA Service 入口，有关联账号时弹窗提示"先删除关联账号"。

### 4.3 Accounts 页面变更

#### 创建入口

Add Account 按钮点击后弹出来源选择：

```
Choose Source

+-----------------------------+  +-----------------------------+
|  OpenAI-Compatible          |  |  CPA Provider Channel       |
|                             |  |                             |
|  Direct connection to any   |  |  Route through a CPA        |
|  OpenAI-compatible API.     |  |  service provider.          |
+-----------------------------+  +-----------------------------+
```

选择 CPA Provider Channel 时，如果尚未配置 CPA Service，提示跳转到 CPA Service 页面。

#### OpenAI-Compatible 表单

与 v1 基本相同，新增 Provider 选择器（Phase A 仅作为标注，Phase C 做自动填充）：

```
Provider       [Select...]         (optional, dropdown)
Label          [________________]
Base URL       [________________]
API Key        [________________]
Model Allowlist [________________]  (comma-separated)
Quota          [total] / [used] [unit]
Notes          [________________]
```

#### CPA Provider Channel 表单

```
CPA Service    [Local CPA]         (首版只有一个，自动选中并灰显)
Provider       [codex ▾]           (下拉列表)
Label          [________________]  (auto-fill: "CPA - Codex")
Model Allowlist [________________]  (comma-separated, optional)
Notes          [________________]
```

Provider 下拉列表选项（首版硬编码）：

| 值 | 显示 |
|---|---|
| codex | Codex (ChatGPT Plus/Pro) |
| claude | Claude |
| gemini | Gemini |
| gemini-cli | Gemini CLI |
| vertex | Vertex AI |
| aistudio | AI Studio |
| openai | OpenAI |
| qwen | Qwen |
| kimi | Kimi |
| iflow | iFlow (GLM) |
| antigravity | Antigravity |

如果 CPA Service 测试连接成功返回了 providers 列表，可动态渲染替代硬编码。

#### 列表变更

新增 Source 列，使用 Badge 标签区分：

| source_kind | Badge 样式 | 示例 |
|---|---|---|
| openai_compat | 默认/淡灰 | `OpenAI compat` |
| cpa | 强调/蓝色 | `CPA - codex` |

CPA 账号 Badge 上的 tooltip 说明："Provider channel - aggregated view of all CPA credentials for this provider"

Runtime 列显示：

- openai_compat: base_url
- cpa: CPA service label + provider name

Quota 列：

- openai_compat: 保持 v1 显示
- cpa: 显示 `-`（不适用）

#### 编辑差异

cpa 账号的编辑 Dialog 只展示可编辑字段（label、notes、model_allowlist），不可编辑字段以只读方式展示。

### 4.4 Overview 页面变更

新增 CPA 状态卡片（单个汇总块，不是"每个 service 一张卡"）：

```
+---------------------------+
|  CPA Service              |
|  Connected (healthy)      |
|  2 provider channels      |
|  Last check: 2 min ago    |
+---------------------------+
```

无 CPA Service 时不显示此卡片。

### 4.5 Usage 页面变更

过滤器区域新增 Source 下拉：

```
Source  [All ▾]    Token  [All ▾]    From  [____]    To  [____]
         +-- All
         +-- OpenAI Compatible
         +-- CPA
```

### 4.6 TypeScript 类型扩展

```typescript
// 新增
interface CpaService {
  id: number
  label: string
  base_url: string
  api_key_set: boolean
  api_key_masked: string
  enabled: boolean
  status: 'unknown' | 'healthy' | 'error'
  last_checked_at: string | null
  last_error: string
  created_at: string
  updated_at: string
}

interface CpaServiceTestResult {
  reachable: boolean
  latency_ms: number
  providers: string[]
  error: string
}

// 扩展
interface Account {
  // ...v1 fields...
  source_kind: 'openai_compat' | 'cpa'
  provider: string
  cpa_service_id: number | null
  cpa_provider: string
  cpa_account_key: string
  runtime: {
    base_url: string
    auth_mode: string
  } | null
}

interface Overview {
  // ...v1 fields...
  cpa_status: {
    connected: boolean
    label: string
    status: string
    accounts_total: number
    accounts_healthy: number
    accounts_error: number
    last_checked_at: string | null
  } | null
  accounts_by_source: {
    openai_compat: number
    cpa: number
  }
}
```

---

## 5. 运行时

### 5.1 请求转发分支

路由选中 account 后，根据 source_kind 分支：

#### source_kind=openai_compat

保持 v1 现有行为：

```
base_url = account.base_url
api_key  = account.api_key
target   = {base_url}/chat/completions
auth     = "Bearer {api_key}"
```

#### source_kind=cpa

从缓存获取关联的 CPA Service：

```
cpa_service = cache.GetCpaService(account.cpa_service_id)
base_url    = cpa_service.base_url + "/api/provider/" + account.cpa_provider + "/v1"
api_key     = cpa_service.api_key
target      = {base_url}/chat/completions
auth        = "Bearer {api_key}"
```

转发后的行为（抄写响应、解析 usage、记录日志）完全一致，无需分支。

### 5.2 路由缓存扩展

RoutingCache 新增 CpaServices 快照：

```go
type Snapshot struct {
    // ...v1 fields...
    CpaServices map[int64]*CpaService
}
```

缓存失效触发条件新增：CPA Service 的 CRUD 操作。

### 5.3 健康检查

#### openai_compat 账号

保持 v1 行为：GET {base_url}/models

#### cpa 账号

1. 检查关联的 CPA Service 是否 enabled 且 status=healthy
2. 如果 service 不健康，标记账号 status=error，last_error="CPA service unreachable"
3. 如果 service 健康，请求 {cpa_service.base_url}/api/provider/{cpa_provider}/v1/models
4. 成功返回模型列表 -> status=healthy
5. 失败 -> status=error 并记录错误详情

#### CPA Service 自身

单独的健康检查循环（每 60s，与 account 健康检查同频）：

1. GET {base_url}/healthz
2. 成功 -> status=healthy
3. 失败 -> status=error

### 5.4 日志

request_logs 新增 source_kind 字段，值跟随被选中的 account。

现有日志维度（account_id、pool_id、target_model）保持不变。

---

## 6. 部署

### 6.1 配置

新增配置项（Phase A 只读 cpa-auth，Phase B 读写）：

```yaml
# lune.yaml
cpa_auth_dir: /app/cpa-auth
```

环境变量：LUNE_CPA_AUTH_DIR

默认值：无。不配置时 Phase B 的导入/登录功能不可用，Phase A 不受影响。

### 6.2 Docker Compose 变更

```yaml
services:
  lune:
    # ...existing config...
    volumes:
      - lune-data:/app/data
      - cpa-auth:/app/cpa-auth    # 新增

  lune-upstream-node:
    image: eceasy/cli-proxy-api:latest
    # ...existing config...
    volumes:
      - cpa-auth:/app/cpa-auth    # 共享

volumes:
  lune-data:
  cpa-auth:     # 新增，Lune 和 CPA 共享
```

---

## 7. 实施清单

按依赖顺序：

### 7.1 后端

1. store: 新增 v2 迁移脚本
2. store: 新增 CpaService 模型与 CRUD（含单实例约束）
3. store: 扩展 Account 模型与创建/更新逻辑
4. store: 扩展 RoutingCache 加入 CpaServices 快照
5. admin: 新增 cpa_service handler（单实例 CRUD + test + enable/disable）
6. admin: 扩展 account handler（create/update 按 source_kind 分支）
7. admin: 扩展 overview 响应
8. admin: 扩展 usage 过滤参数
9. admin: 扩展 export 包含 cpa_services
10. gateway: 转发逻辑按 source_kind 分支
11. gateway: 日志记录 source_kind
12. health: 新增 CPA Service 健康检查
13. health: CPA 账号健康检查逻辑
14. httpserver: 注册 CPA Service 路由

### 7.2 前端

1. types.ts: 新增 CpaService、CpaServiceTestResult 接口，扩展 Account、Overview
2. App.tsx: 新增 /admin/cpa-service 路由
3. Shell.tsx: 侧边栏新增 CPA Service 入口
4. CpaServicePage.tsx: 新页面（单实例设置 + 测试连接 + 关联账号列表）
5. AccountsPage.tsx: 来源选择入口 + CPA 表单 + 列表 Source 列 + 编辑差异
6. OverviewPage.tsx: CPA 状态汇总块
7. UsagePage.tsx: Source 过滤器

---

## 8. 验收场景

### 8.1 OpenAI-Compatible 账号（v1 兼容）

- [ ] 可以继续创建 base_url + api_key 账号
- [ ] 已有 v1 账号迁移后正常工作，source_kind=openai_compat
- [ ] 加入 pool 后 route 正常使用
- [ ] 健康检查正常

### 8.2 CPA Service

- [ ] 可创建（仅 1 个）、编辑、启停
- [ ] 已存在 1 个时再创建返回 409
- [ ] 测试连接返回状态与可用 providers
- [ ] 健康检查定期更新 status
- [ ] 有关联账号时拒绝删除（409）
- [ ] 无关联账号时可正常删除

### 8.3 CPA Provider Channel

- [ ] 选择 CPA Service + provider 创建账号
- [ ] 同一 service + provider 不允许重复创建
- [ ] 账号列表正确显示 Source badge 和 Runtime 信息
- [ ] Source badge tooltip 说明这是 provider channel
- [ ] 编辑 Dialog 只展示可编辑字段
- [ ] quota 字段不展示

### 8.4 运行时请求

- [ ] cpa 账号被选中时，请求转发到 CPA /api/provider/{provider}/v1/chat/completions
- [ ] Bearer token 使用 CPA Service 的 api_key
- [ ] streaming 和 non-streaming 均正常
- [ ] usage / logs 按本地 account 维度记录，source_kind=cpa
- [ ] pool 可混合 openai_compat 和 cpa 账号
- [ ] 重试逻辑正常（failover 到同 pool 其他账号）

### 8.5 健康与故障

- [ ] CPA Service 宕机 -> 其下所有 cpa 账号标记 status=error
- [ ] CPA Service 恢复 -> 下次检查自动恢复 status=healthy
- [ ] 单个 CPA provider 不可用 -> 仅该账号 error
- [ ] openai_compat 账号不受 CPA 状态影响

### 8.6 Overview 与 Usage

- [ ] Overview 展示 CPA Service 连接状态和 provider channel 汇总
- [ ] 无 CPA Service 时不显示 CPA 区域
- [ ] Usage 支持按 source_kind 过滤
