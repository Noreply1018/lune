# Phase C: 体验增强与分析

> 独立于 CPA 改造，提升 OpenAI-compatible 侧的使用体验和全局分析能力。可与 Phase A/B 并行或在其之后实施。

## 1. Provider 模板

### 1.1 目标

创建 openai_compat 账号时，选择 Provider 后自动填充 base_url，减少手动输入。

### 1.2 预置模板

| Provider | base_url |
|---|---|
| OpenAI | `https://api.openai.com/v1` |
| DeepSeek | `https://api.deepseek.com/v1` |
| Groq | `https://api.groq.com/openai/v1` |
| Mistral | `https://api.mistral.ai/v1` |
| Together AI | `https://api.together.xyz/v1` |
| OpenRouter | `https://openrouter.ai/api/v1` |
| Moonshot | `https://api.moonshot.cn/v1` |
| Custom | 用户手动输入 |

### 1.3 实现方式

- **前端硬编码**。模板数据以常量数组存在前端，不需要后端 API
- Provider 下拉选择后自动填充 base_url
- 选择 Custom 时 base_url 为空，用户手填
- 用户可在自动填充后手动修改 base_url（覆盖模板值）
- Label 自动建议：如选择 DeepSeek 则 Label 默认建议 "DeepSeek"

### 1.4 前端变更

AccountsPage 的 OpenAI-Compatible 表单中，在 Label 上方增加 Provider 下拉：

```
Provider       [Select provider... v]
Label          [________________]        <- auto-fill from provider
Base URL       [________________]        <- auto-fill from provider
API Key        [________________]
...
```

---

## 2. 一键测试连接

### 2.1 目标

在创建/编辑 openai_compat 账号时，一键验证 base_url + api_key 是否有效。

### 2.2 API

| Method | Path | 说明 |
|---|---|---|
| POST | /admin/api/accounts/test-connection | 测试 OpenAI-compatible 连接 |

请求：

```json
{
  "base_url": "https://api.openai.com/v1",
  "api_key": "sk-..."
}
```

响应（成功）：

```json
{
  "data": {
    "reachable": true,
    "latency_ms": 230,
    "models": ["gpt-4o", "gpt-4.1", "gpt-4-turbo", "o3-mini"],
    "error": ""
  }
}
```

响应（失败）：

```json
{
  "data": {
    "reachable": false,
    "latency_ms": 0,
    "models": [],
    "error": "401 Unauthorized"
  }
}
```

### 2.3 测试逻辑

1. 向 `{base_url}/models` 发送 GET 请求，携带 `Authorization: Bearer {api_key}`
2. 成功则解析模型列表返回
3. 失败则返回错误详情（状态码、网络错误等）
4. 超时上限：10 秒

### 2.4 前端交互

表单底部增加 Test Connection 按钮：

```
[Test Connection]   <- 仅在 base_url 和 api_key 均非空时可点击
```

- 点击后按钮变为 loading 状态
- 成功：green toast "Connected (230ms) - 4 models available"
- 失败：red toast 错误详情

**额外功能**：测试成功后，可将返回的模型列表作为 model_allowlist 的候选项提供给用户选择。

---

## 3. 环境变量代码片段增强

### 3.1 目标

在 Tokens 页面，创建 access token 成功后，展示多种格式的环境变量配置，方便用户复制。

### 3.2 代码片段格式

#### .env

```
OPENAI_API_KEY=sk-lune-xxxxxx
OPENAI_BASE_URL=http://your-lune-host:7788/v1
```

#### Shell export

```bash
export OPENAI_API_KEY="sk-lune-xxxxxx"
export OPENAI_BASE_URL="http://your-lune-host:7788/v1"
```

#### Python

```python
from openai import OpenAI

client = OpenAI(
    api_key="sk-lune-xxxxxx",
    base_url="http://your-lune-host:7788/v1",
)
```

#### Node.js

```javascript
import OpenAI from 'openai';

const client = new OpenAI({
  apiKey: 'sk-lune-xxxxxx',
  baseURL: 'http://your-lune-host:7788/v1',
});
```

#### curl

```bash
curl http://your-lune-host:7788/v1/chat/completions \
  -H "Authorization: Bearer sk-lune-xxxxxx" \
  -H "Content-Type: application/json" \
  -d '{"model": "gpt-4o", "messages": [{"role": "user", "content": "Hello"}]}'
```

### 3.3 前端交互

Token 创建成功的 Dialog 中，新增 tab 切换：

```
+--------------------------------------+
|  Token Created                       |
|                                      |
|  sk-lune-xxxxxx        [Copy Token]  |
|                                      |
|  Quick Setup:                        |
|  [.env] [Shell] [Python] [Node] [curl] |
|  +------------------------------+   |
|  | export OPENAI_API_KEY=...     |   |
|  | export OPENAI_BASE_URL=...    |   |
|  +------------------------------+   |
|                          [Copy]      |
|                                      |
|                  [Done]              |
+--------------------------------------+
```

Lune 的 host 地址从当前浏览器 window.location 自动检测。

---

## 4. 内置 Playground

### 4.1 目标

提供一个内置的聊天界面，用于快速测试 Lune 网关端到端的可用性。

### 4.2 页面路由

```
/admin/playground
```

侧边栏新增入口，放在 Observe 分组中：

```
Observe
  +-- Overview        /admin
  +-- Usage           /admin/usage
  +-- Playground      /admin/playground    <- new
```

### 4.3 界面设计

```
+----------------------------------------------+
|  Playground                                  |
|                                              |
|  Model  [gpt-4o v]     Token [default v]     |
|  Stream [x]                                  |
|                                              |
|  +--------------------------------------+    |
|  |                                      |    |
|  |  (chat messages here)                |    |
|  |                                      |    |
|  +--------------------------------------+    |
|                                              |
|  [Type a message...                ] [Send]  |
|                                              |
|  Latency: 1.2s  Tokens: 42 in / 128 out     |
+----------------------------------------------+
```

### 4.4 功能要求

- **模型选择**：从 GET /v1/models 获取可用模型列表
- **Token 选择**：从 GET /admin/api/tokens 获取，用选中 token 的实际值作为 Bearer
- **Streaming**：默认开启 SSE streaming
- **延迟显示**：请求结束后显示 TTFB 和总延迟
- **Token 用量**：显示 prompt_tokens / completion_tokens
- **多轮对话**：保持对话历史，支持 Clear 清空
- **请求通过 Lune 网关**：直接调用 /v1/chat/completions，不走 admin API

### 4.5 实现说明

- 纯前端实现，不需要新增后端 API
- 前端直接调用 Lune 的 OpenAI 兼容端点
- Token 获取方案：Playground 中让用户手动输入 token，或在 Settings 中指定 Playground 默认 token

---

## 5. 成本估算

### 5.1 目标

基于请求日志中的 token 用量，按模型定价估算成本。

### 5.2 定价数据

前端维护一份模型定价表（硬编码，定期更新）：

```typescript
const PRICING: Record<string, { input: number; output: number }> = {
  'gpt-4o':    { input: 2.50, output: 10.00 },     // per 1M tokens
  'gpt-4.1':   { input: 2.00, output: 8.00 },
  'o3-mini':   { input: 1.10, output: 4.40 },
  'claude-sonnet-4-20250514': { input: 3.00, output: 15.00 },
  // ...
};
```

### 5.3 展示位置

- **Usage 页面**：每条请求日志右侧显示估算成本（~$0.003）
- **Usage 页面顶部**：时间范围内的总估算成本汇总
- **Overview 页面**：24h / 7d 估算成本 StatCard

### 5.4 注意事项

- 成本估算仅供参考，不等于实际账单
- CPA 账号的成本通常是订阅制，token 计费不适用，但仍可展示用量
- 未匹配定价的模型显示 `-`
- 前端计算，不存入数据库

---

## 6. 延迟追踪

### 6.1 目标

可视化各模型 / 账号 / 时间段的请求延迟趋势。

### 6.2 后端扩展

新增聚合查询 API：

```
GET /admin/api/usage/latency?model=gpt-4o&period=24h&bucket=1h
```

响应：

```json
{
  "data": {
    "buckets": [
      { "time": "2026-04-13T00:00:00Z", "p50_ms": 800, "p95_ms": 2100, "p99_ms": 4500, "count": 42 },
      { "time": "2026-04-13T01:00:00Z", "p50_ms": 750, "p95_ms": 1900, "p99_ms": 3800, "count": 38 }
    ]
  }
}
```

### 6.3 前端展示

- **Usage 页面**：增加 Latency tab，展示折线图
- **Accounts 页面**：每个账号行可展开显示最近 24h 延迟 Sparkline（迷你折线图）
- 图表库建议使用轻量方案（如 recharts 或纯 SVG/Canvas），避免引入大型依赖

---

## 7. 实施优先级建议

| 优先级 | 功能 | 复杂度 | 价值 |
|---|---|---|---|
| 1 | Provider 模板 | 低（纯前端） | 高 |
| 2 | 一键测试连接 | 低 | 高 |
| 3 | 环境变量代码片段 | 低（纯前端） | 中 |
| 4 | 内置 Playground | 中 | 高 |
| 5 | 成本估算 | 低（纯前端） | 中 |
| 6 | 延迟追踪 | 中（需后端聚合） | 中 |

Provider 模板和测试连接可以与 Phase A 同批实施（仅涉及 openai_compat 侧）。

---

## 8. 验收场景

### 8.1 Provider 模板

- [ ] 选择 Provider 后 base_url 和 Label 自动填充
- [ ] 选择 Custom 后字段为空
- [ ] 自动填充后用户仍可手动修改

### 8.2 测试连接

- [ ] base_url + api_key 有效时成功提示 + 模型列表
- [ ] api_key 无效时失败提示 401
- [ ] base_url 不可达时失败提示网络错误
- [ ] 超时时失败提示超时

### 8.3 环境变量代码片段

- [ ] Token 创建后展示多格式代码片段
- [ ] 代码片段中的 host 地址自动检测
- [ ] 每种格式均可一键复制

### 8.4 Playground

- [ ] 可选择模型和 token
- [ ] 发送消息后收到 streaming 响应
- [ ] 显示延迟和 token 用量
- [ ] 多轮对话正常

### 8.5 成本估算

- [ ] 已知模型的请求日志显示估算成本
- [ ] Usage 页面顶部显示总估算成本
- [ ] 未知模型显示 `-`

### 8.6 延迟追踪

- [ ] 折线图正确展示 p50/p95 延迟
- [ ] 可按模型、时间范围过滤
- [ ] 账号行展示延迟 Sparkline
