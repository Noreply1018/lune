# Phase B: CPA 管理 Adapter

> CPA 是外部镜像（eceasy/cli-proxy-api），不可修改源码。所有管理能力由 Lune 自己实现。

## 1. 架构：Lune CPA Management Adapter

### 1.1 核心约束

CPA（eceasy/cli-proxy-api）是闭源外部镜像，Lune 无法要求 CPA 新增管理 API。因此 Phase B 的所有管理能力由 **Lune 后端自己实现**，通过以下两个接口与 CPA 交互：

1. **cpa-auth 共享目录**（文件级交互）
   - Lune 读取：扫描已有凭据文件，提取账号元信息
   - Lune 写入：设备码登录成功后，将凭据写入新 JSON 文件
   - CPA 读取：热加载目录变更，自动识别新凭据，无需重启
   - CPA 写入：token 自动刷新时更新已有 JSON 文件

2. **CPA 推理接口**（HTTP 级交互，Phase A 已有）
   - /healthz, /v1/models, /v1/chat/completions 等
   - /api/provider/{provider}/v1/... 按 provider 路由
   - Phase B 不新增对 CPA 的 HTTP 调用

### 1.2 Adapter 职责

| 能力 | 实现者 | 接口 |
|---|---|---|
| 设备码登录 | Lune 直接对接 OpenAI OAuth | HTTPS -> auth.openai.com |
| 凭据落盘 | Lune 写入 cpa-auth/ | 文件系统 |
| 凭据扫描/导入 | Lune 读取 cpa-auth/ | 文件系统 |
| 账号元信息解析 | Lune 解析 JSON 文件 | 文件系统 |
| token 自动刷新 | CPA 负责（每 15 分钟） | CPA 内部 |
| 推理转发 | CPA 负责 | HTTP（Phase A 已有） |

### 1.3 不做什么

- 不修改 CPA 源码或镜像
- 不让 Lune 接管 token 刷新（CPA 每 15 分钟自动刷新）
- 不让 Lune 直接调用 OpenAI 推理 API（仍通过 CPA 转发）
- 不实现 CPA 管理 REST API 的 mock（直接用文件系统）

---

## 2. cpa-auth 目录规约

### 2.1 文件格式

每个凭据文件是一个 JSON 文件，文件名格式：

```
{type}-{email}-{plan}.json
```

示例：

```
cpa-auth/
  codex-cartercabrerazwu@mail.com-plus.json
  codex-user2@example.com-pro.json
```

### 2.2 JSON 结构

```json
{
  "access_token": "eyJhbGciOi...",
  "refresh_token": "rt_A_PhPh7r90...",
  "id_token": "eyJhbGciOi...",
  "account_id": "fe9fc78b-1989-4f2a-80a8-18b525cf4d6d",
  "email": "cartercabrerazwu@mail.com",
  "disabled": false,
  "expired": "2026-04-21T15:48:45+08:00",
  "last_refresh": "2026-04-11T15:48:45+08:00",
  "type": "codex"
}
```

字段说明：

| 字段 | 说明 | Lune 是否需要 |
|---|---|---|
| access_token | OAuth access token（JWT） | 写入时需要，不存入 Lune DB |
| refresh_token | OAuth refresh token | 写入时需要，不存入 Lune DB |
| id_token | OIDC id token（JWT） | 写入时需要，不存入 Lune DB |
| account_id | OpenAI 账号 UUID | 读取，存入 cpa_openai_id |
| email | 账号邮箱 | 读取，存入 cpa_email |
| disabled | 是否被禁用 | 读取，存入 cpa_disabled |
| expired | 凭据过期时间 | 读取，存入 cpa_expired_at |
| last_refresh | 最近刷新时间 | 读取，存入 cpa_last_refresh_at |
| type | provider 类型（codex） | 读取，用于 cpa_provider 匹配 |

**安全原则：Lune 数据库中绝不存储 access_token / refresh_token / id_token 明文。**

### 2.3 部署约束

Lune 和 CPA 必须挂载同一个 cpa-auth 目录：

```yaml
# docker-compose.yml
volumes:
  cpa-auth:   # named volume, shared

services:
  lune:
    volumes:
      - cpa-auth:/app/cpa-auth
    environment:
      LUNE_CPA_AUTH_DIR: /app/cpa-auth

  lune-upstream-node:  # CPA
    volumes:
      - cpa-auth:/app/cpa-auth
```

本地开发时可使用宿主机目录：

```yaml
# lune.yaml
cpa_auth_dir: ./cpa-auth
```

### 2.4 读写时序

```
Lune                           cpa-auth/                    CPA
  |                               |                           |
  |  device code login success    |                           |
  |  --> write JSON file -------->|                           |
  |                               |  hot-reload detect  ----->|
  |                               |                    ready  |
  |                               |                           |
  |                               |  CPA auto-refresh (15m)  |
  |                               |<-- update JSON file ------|
  |  health check scan            |                           |
  |  <-- read JSON file ---------|                           |
```

---

## 3. 数据模型扩展

### 3.1 Account 新增 CPA 元信息字段

在 Phase A 基础上扩展 accounts 表（schema_version 从 2 升至 3）：

```sql
ALTER TABLE accounts ADD COLUMN cpa_email           TEXT NOT NULL DEFAULT '';
ALTER TABLE accounts ADD COLUMN cpa_plan_type       TEXT NOT NULL DEFAULT '';
ALTER TABLE accounts ADD COLUMN cpa_openai_id       TEXT NOT NULL DEFAULT '';
ALTER TABLE accounts ADD COLUMN cpa_expired_at      TEXT;
ALTER TABLE accounts ADD COLUMN cpa_last_refresh_at TEXT;
ALTER TABLE accounts ADD COLUMN cpa_disabled        INTEGER NOT NULL DEFAULT 0;
```

### 3.2 cpa_account_key 的正式启用

Phase A 中预留的 cpa_account_key 字段在此阶段正式使用。

命名规则（与文件名一致，去掉 .json）：

```
codex-cartercabrerazwu@mail.com-plus
```

唯一约束升级：

```sql
-- 替换 Phase A 的 provider 级唯一约束
DROP INDEX IF EXISTS idx_accounts_cpa_unique;

-- 改为 account_key 级唯一约束
CREATE UNIQUE INDEX IF NOT EXISTS idx_accounts_cpa_key_unique
    ON accounts(cpa_service_id, cpa_account_key)
    WHERE source_kind = 'cpa' AND cpa_account_key != '';
```

### 3.3 粒度变化

Phase A: 一个 CPA account = 一个 provider（如 codex），CPA 内部均衡多凭据。

Phase B: 一个 CPA account = 一个具体的凭据文件（如 codex-user@mail.com-plus）。

同一个 CPA Service 下可能有：
- codex-user1@mail.com-plus
- codex-user2@mail.com-pro
- claude-user1@mail.com-plus

Lune 的 pool 可以同时包含这些账号，实现 Lune 侧的负载均衡。

### 3.4 与 Phase A 的兼容

- Phase A 创建的 CPA 账号（cpa_account_key 为空）继续正常工作
- 两种粒度可共存：provider channel 让 CPA 内部均衡，account 粒度让 Lune 侧均衡
- 前端 CPA 创建入口变为三选一：
  1. Provider Channel（Phase A 已有）
  2. Login with Device Code（新增）
  3. Import Existing Account（新增）

---

## 4. 设备码登录

### 4.1 实现方式：Lune 直接对接 OpenAI OAuth

Lune 后端直接实现 OAuth 2.0 Device Authorization Grant (RFC 8628)，对接 auth.openai.com。

不依赖 CPA 的任何接口。登录成功后 Lune 将凭据 JSON 写入 cpa-auth/ 共享目录，CPA 热加载自动识别。

### 4.2 OAuth 端点

| 步骤 | 端点 | 方法 | 说明 |
|---|---|---|---|
| 1. 请求设备码 | https://auth.openai.com/api/accounts/deviceauth/usercode | POST | 获取 device_code + user_code |
| 2. 用户验证页 | https://auth.openai.com/codex/device | - | 用户在浏览器打开并输入 user_code |
| 3. 轮询换 token | https://auth.openai.com/api/accounts/deviceauth/token | POST | 用 device_code 轮询直到授权完成 |
| 4. Token 刷新 | https://auth.openai.com/oauth/token | POST | CPA 负责，Lune 不实现 |

#### 请求设备码参数

```json
{
  "client_id": "app_EMoamEEZ73f0CkXaXp7hrann",
  "scope": "openid email profile offline_access"
}
```

client_id 是 OpenAI 官方 Codex CLI 的 client_id。

#### 轮询换 token 参数

```json
{
  "device_code": "...",
  "grant_type": "urn:ietf:params:oauth:grant-type:device_code",
  "client_id": "app_EMoamEEZ73f0CkXaXp7hrann"
}
```

#### 成功响应

```json
{
  "access_token": "eyJhbGciOi...",
  "refresh_token": "rt_...",
  "id_token": "eyJhbGciOi...",
  "expires_in": 863999,
  "token_type": "Bearer"
}
```

### 4.3 凭据落盘流程

登录成功后，Lune 需要：

1. 从 access_token JWT 中解析账号元信息：
   - email: 从 `https://api.openai.com/profile` claim 提取
   - plan_type: 从 `https://api.openai.com/auth` claim 的 chatgpt_plan_type 提取
   - account_id: 从 `https://api.openai.com/auth` claim 的 chatgpt_account_id 提取
2. 计算过期时间：`expired = now + expires_in`
3. 构造 JSON 文件：

```json
{
  "access_token": "eyJhbGciOi...",
  "refresh_token": "rt_...",
  "id_token": "eyJhbGciOi...",
  "account_id": "fe9fc78b-...",
  "email": "user@mail.com",
  "disabled": false,
  "expired": "2026-04-21T15:48:45+08:00",
  "last_refresh": "2026-04-13T12:00:00+08:00",
  "type": "codex"
}
```

4. 写入文件：`{cpa_auth_dir}/codex-{email}-{plan}.json`
5. CPA 热加载自动识别新文件
6. Lune 创建本地 account 记录（不存 token 明文，只存元信息）

### 4.4 用户路径

1. 打开 Accounts
2. 选择 Add Account -> CPA Provider Channel -> Login with Device Code
3. 前端向 Lune 发起登录请求
4. 前端显示 verification_uri 与 user_code
5. 用户在浏览器完成授权
6. 前端轮询 Lune，Lune 轮询 auth.openai.com
7. 登录成功后 Lune 写入凭据 + 创建本地 account
8. 用户可直接把该 account 加入 pool

### 4.5 Lune API

| Method | Path | 说明 |
|---|---|---|
| POST | /admin/api/accounts/cpa/login-sessions | 发起设备码登录 |
| GET | /admin/api/accounts/cpa/login-sessions/{id} | 查询登录状态 |
| POST | /admin/api/accounts/cpa/login-sessions/{id}/cancel | 取消登录 |

#### 创建 Session 请求

```json
{
  "service_id": 1
}
```

#### 创建 Session 响应

```json
{
  "data": {
    "id": "login_01",
    "status": "pending",
    "verification_uri": "https://auth.openai.com/codex/device",
    "user_code": "ABCD-EFGH",
    "expires_at": "2026-04-13T13:00:00Z",
    "poll_interval_seconds": 5
  }
}
```

#### 轮询响应（成功）

```json
{
  "data": {
    "id": "login_01",
    "status": "succeeded",
    "account_id": 12,
    "account": {
      "id": 12,
      "label": "codex - user@mail.com (plus)",
      "source_kind": "cpa",
      "cpa_provider": "codex",
      "cpa_email": "user@mail.com",
      "cpa_plan_type": "plus"
    }
  }
}
```

#### 轮询响应（失败）

```json
{
  "data": {
    "id": "login_01",
    "status": "failed",
    "error_code": "authorization_denied",
    "error_message": "User denied authorization"
  }
}
```

### 4.6 状态机

```
pending -> authorized -> succeeded
    |          |
    +-> expired  +-> failed
    +-> cancelled
```

| 状态 | 说明 |
|---|---|
| pending | 已创建 session，等待用户授权 |
| authorized | 用户已完成授权，Lune 正在处理凭据 |
| succeeded | 凭据已落盘 + 本地 account 已创建 |
| expired | 设备码过期 |
| failed | 登录/落盘/建号任一环节失败 |
| cancelled | 用户主动取消 |

succeeded 之前如果本地 account 创建失败，状态进入 failed，已写入的凭据文件保留（CPA 仍可使用）。

### 4.7 Session 存储与约束

- **内存存储**（sync.Map 或带锁的 map）
- 同一时刻每个 CPA Service 仅允许 **1 个 active session**（pending 或 authorized 状态）
  - 新发起时如果已有 active session，返回 409 Conflict
  - 用户可先 cancel 再重新发起
- Lune 重启后所有 session 丢失
- 前端轮询收到 404 应展示"登录会话已失效，请重新开始"

### 4.8 设备码登录 UI

```
+--------------------------------------+
|  Device Code Login                   |
|                                      |
|  Open this URL:                      |
|  https://auth.openai.com/codex/...   |
|  [Open Login Page]                   |
|                                      |
|  Enter this code:                    |
|  +------------------+               |
|  |   ABCD-EFGH      |  [Copy]       |
|  +------------------+               |
|                                      |
|  Waiting for authorization...        |
|  Expires in 8:42                     |
|                                      |
|             [Cancel]                 |
+--------------------------------------+
```

状态区变化：
- pending: "Waiting for authorization..." + 倒计时
- authorized: "Authorized, finalizing account..."
- succeeded: 展示账号摘要（email、plan、provider），按钮变为 Done
- failed / expired: 错误提示 + Retry 按钮

---

## 5. 导入已有 CPA 账号

### 5.1 实现方式：Lune 扫描 cpa-auth 目录

Lune 直接读取 cpa-auth/ 目录中的 JSON 文件，解析账号元信息，供用户选择导入。

不依赖 CPA 的任何管理接口。

### 5.2 Lune API

| Method | Path | 说明 |
|---|---|---|
| GET | /admin/api/cpa/service/remote-accounts | 扫描 cpa-auth 目录列出账号 |
| POST | /admin/api/accounts/cpa/import | 导入一个 CPA 账号 |

#### 扫描响应

```json
{
  "data": [
    {
      "account_key": "codex-user@mail.com-plus",
      "email": "user@mail.com",
      "plan_type": "plus",
      "provider": "codex",
      "account_id": "fe9fc78b-...",
      "expired_at": "2026-04-21T07:48:45Z",
      "disabled": false,
      "already_imported": false
    },
    {
      "account_key": "codex-user2@mail.com-pro",
      "email": "user2@mail.com",
      "plan_type": "pro",
      "provider": "codex",
      "account_id": "...",
      "expired_at": "2026-05-01T00:00:00Z",
      "disabled": false,
      "already_imported": true
    }
  ]
}
```

扫描逻辑：

1. 列出 cpa_auth_dir 下所有 .json 文件
2. 逐个解析 JSON，提取元信息
3. 与本地 accounts 表比对（cpa_service_id + cpa_account_key），标记 already_imported
4. 忽略解析失败的文件

already_imported 为 true 的条目：Lune 本地已导入该 account_key。

#### 导入请求

```json
{
  "service_id": 1,
  "account_key": "codex-user@mail.com-plus",
  "label": "My Codex",
  "enabled": true,
  "notes": "",
  "model_allowlist": []
}
```

后端导入逻辑：

1. 读取 cpa-auth/{account_key}.json 文件
2. 解析元信息（email、plan_type、account_id、expired、disabled、type）
3. 创建本地 account 记录：
   - source_kind = "cpa"
   - cpa_service_id = 请求中的 service_id
   - cpa_provider = JSON 中的 type 字段
   - cpa_account_key = 请求中的 account_key
   - cpa_email / cpa_plan_type / cpa_openai_id / cpa_expired_at / cpa_disabled = 从 JSON 提取
4. 不存储 token 明文

### 5.3 导入 UI

```
+----------------------------------------------+
|  Import CPA Account                          |
|                                              |
|  Available Accounts:                         |
|  +--------------------------------------+    |
|  | [ ] user@mail.com - plus - codex     |    |
|  |     Expires: Apr 21, 2026            |    |
|  | [x] user2@mail.com - pro - codex     |    |
|  |     Expires: May 01, 2026            |    |
|  | [*] user3@mail.com - plus - claude   |    |
|  |     Already imported                  |    |
|  +--------------------------------------+    |
|                                              |
|  Label  [________________]                   |
|                                              |
|           [Cancel]  [Import]                 |
+----------------------------------------------+
```

已导入的账号灰显且不可选。

---

## 6. 账号元数据同步

### 6.1 同步时机

- **健康检查时**（每 60s）：对每个 cpa 账号（有 cpa_account_key 的），读取对应的 cpa-auth JSON 文件
- **手动刷新**：前端可触发单个账号的立即同步

### 6.2 同步字段

从 JSON 文件更新到本地 DB：

| 字段 | 说明 |
|---|---|
| cpa_expired_at | 凭据过期时间（CPA 刷新后会更新） |
| cpa_last_refresh_at | 最近 token 刷新时间 |
| cpa_disabled | 是否被禁用 |

### 6.3 异常处理

- JSON 文件不存在（被手动删除或 CPA 清理了）-> 标记 status=error, last_error="Credential file not found"
- JSON 解析失败 -> 保持当前元数据不变，标记 status=error
- cpa_auth_dir 未配置 -> 跳过文件级同步，只做 HTTP 级健康检查

---

## 7. 到期预警

### 7.1 前端展示

Accounts 列表中，对有 cpa_expired_at 的 CPA 账号增加到期状态指示：

| 条件 | 展示 |
|---|---|
| cpa_expired_at 距今 > 7 天 | 正常显示 |
| cpa_expired_at 距今 <= 7 天 | 黄色 Expiring soon Badge |
| cpa_expired_at 距今 <= 24 小时 | 红色 Expiring today Badge |
| cpa_expired_at 已过期 | 红色 Expired Badge |

注意：CPA 每 15 分钟自动刷新 token，所以 expired_at 在正常情况下会持续延期。如果看到过期警告，通常说明 CPA 的自动刷新出了问题。

### 7.2 Overview 集成

Overview 页面的 CPA 状态块中增加到期提示：

```
+---------------------------+
|  CPA Service              |
|  Connected (healthy)      |
|  4 accounts               |
|  1 expiring soon          |  <-- 如果有即将到期的
|  Last check: 2 min ago    |
+---------------------------+
```

---

## 8. 批量同步

### 8.1 Sync All

CPA Service 页面增加 Sync from CPA 按钮：

1. 扫描 cpa-auth/ 目录全部凭据
2. 与本地已导入的比对
3. 展示差异：新增 / 已导入 / 本地有但文件已消失
4. 用户勾选需要导入的新账号
5. 批量创建本地 account

### 8.2 API

```
POST /admin/api/accounts/cpa/import/batch
```

请求：

```json
{
  "service_id": 1,
  "account_keys": [
    "codex-user@mail.com-plus",
    "claude-user@mail.com-plus"
  ]
}
```

响应：

```json
{
  "data": {
    "imported": 2,
    "skipped": 0,
    "errors": []
  }
}
```

---

## 9. 验收场景

### 9.1 设备码登录

- [ ] 发起登录 session，前端显示 verification_uri + user_code
- [ ] 轮询到成功、失败、过期、取消各状态
- [ ] 成功后凭据 JSON 写入 cpa-auth/ 目录
- [ ] CPA 热加载识别新凭据（不重启）
- [ ] Lune 自动创建本地 account（含 email、plan_type 等元信息）
- [ ] DB 中不存储 access_token / refresh_token / id_token 明文
- [ ] 同一 CPA Service 不允许同时存在 2 个 active session
- [ ] Lune 重启后 pending session 不再可用，前端收到 404 提示重新开始
- [ ] 登录成功但建号失败时状态为 failed，凭据文件保留

### 9.2 导入

- [ ] 扫描 cpa-auth/ 目录列出所有凭据文件
- [ ] 已导入的标记为不可选
- [ ] 导入成功后本地 account 创建正确
- [ ] 同一 service_id + account_key 不重复导入
- [ ] cpa_auth_dir 未配置时返回明确错误

### 9.3 元数据同步

- [ ] 健康检查时自动读取 JSON 文件更新 cpa_expired_at 等字段
- [ ] 凭据文件被删除时本地标记 error
- [ ] cpa_auth_dir 未配置时跳过文件级同步

### 9.4 到期预警

- [ ] 列表中到期时间 <= 7 天的显示黄色警告
- [ ] 列表中到期时间 <= 24 小时的显示红色警告
- [ ] Overview 展示即将到期的账号计数

### 9.5 批量同步

- [ ] 可扫描 cpa-auth/ 全部凭据并识别差异
- [ ] 可批量导入选中的账号
