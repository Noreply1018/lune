# Lune v2 Ideas (Draft)

> 这是 v2 创新点的草稿收集。所有内容都以 v1 完成为前提，不阻塞 v1 交付。

## 状态：草稿 / 头脑风暴

以下功能按预估价值和复杂度排列，不代表实现顺序。

---

## 1. Provider 模板

**问题**：添加账号时需要手动输入 `base_url`，容易打错，还要去查各家的 API 地址。

**方案**：内置主流 provider 模板，创建账号时一键选择，自动填充 `base_url`。

预置模板：

| Provider | base_url |
|---|---|
| OpenAI | `https://api.openai.com/v1` |
| Anthropic (OpenAI compat) | `https://api.anthropic.com/v1` |
| DeepSeek | `https://api.deepseek.com/v1` |
| Groq | `https://api.groq.com/openai/v1` |
| Mistral | `https://api.mistral.ai/v1` |
| Together AI | `https://api.together.xyz/v1` |
| OpenRouter | `https://openrouter.ai/api/v1` |
| Moonshot | `https://api.moonshot.cn/v1` |
| 自定义 | 用户手动输入 |

**实现复杂度**：低。纯前端下拉框 + 预填充逻辑，后端无改动。

**UI 草图**：

```
Provider:  [OpenAI ▼]          ← 选择后自动填充下面的 base_url
Base URL:  [https://api.openai.com/v1]  (可编辑覆盖)
API Key:   [sk-...                    ]
```

---

## 2. 环境变量代码片段

**问题**：创建 token 后，用户还要手动拼 `OPENAI_BASE_URL` 和 `OPENAI_API_KEY`，容易遗漏。

**方案**：Token 创建成功后，一键复制完整的环境变量配置。

**注意**：这个功能已经写进了 v1 的 frontend spec（Token 创建对话框里的 "Quick Setup" 代码块）。如果 v1 来不及做或需要增强，v2 可以扩展更多格式。

v2 扩展方向：

- 多种格式输出：`.env` 文件格式、`export` shell 命令、Python `os.environ` 赋值、JSON config
- 针对不同框架的模板：Next.js、Python、Node.js
- 复制为 curl 测试命令

```
格式: [.env ▼]

# .env
OPENAI_BASE_URL=http://127.0.0.1:7788/v1
OPENAI_API_KEY=sk-lune-a8f2c1d9e3b7...4f2a

[Copy]
```

**实现复杂度**：低。纯前端。

---

## 3. 内置 Playground

**问题**：配好账号和路由后，不确定能不能用。要开另一个工具（curl、ChatGPT 客户端）发请求测试。

**方案**：Admin UI 里内嵌一个极简聊天界面，选模型、发消息、看响应。

**功能范围**：

- 模型选择器（下拉，来自 /v1/models）
- 单轮对话输入框
- 流式响应展示
- 显示 token 用量（从 usage 字段）
- 显示延迟
- 使用内部直连，不走 access token 鉴权（admin 级别权限）

**UI 草图**：

```
┌─────────────────────────────────────────────────┐
│  Playground                                     │
│                                                 │
│  Model: [gpt-4o ▼]                              │
│                                                 │
│  ┌─────────────────────────────────────────┐    │
│  │                                         │    │
│  │ User: 你好，请用一句话介绍自己           │    │
│  │                                         │    │
│  │ Assistant: 我是一个AI助手，可以帮助你    │    │
│  │ 回答问题、写代码和完成各种任务。         │    │
│  │                                         │    │
│  │ ─────────────────────────────────        │    │
│  │ Tokens: 12 in / 28 out  Latency: 1.2s   │    │
│  │                                         │    │
│  └─────────────────────────────────────────┘    │
│                                                 │
│  ┌────────────────────────────────┐ [Send]      │
│  │ Type a message...              │             │
│  └────────────────────────────────┘             │
└─────────────────────────────────────────────────┘
```

**实现复杂度**：中。需要前端 SSE 流式渲染 + 后端新增一个不走 token 鉴权的内部 chat 端点。

---

## 4. 成本估算仪表盘

**问题**：知道用了多少 token，但不知道花了多少钱。各家 provider 定价不同，手动算很麻烦。

**方案**：

1. 在 Account 上增加一个可选的 `pricing` 配置（每百万 token 单价，区分 input/output）
2. Usage 页面基于 pricing 自动计算成本

**Account 新增字段**：

```json
{
  "pricing": {
    "input_per_million": 2.50,
    "output_per_million": 10.00,
    "currency": "USD"
  }
}
```

**UI 增强**：

- Usage 页面增加 "Estimated Cost" 列
- Overview 页面增加 "Estimated cost (24h)" 统计卡片

```
┌──────────┐ ┌──────────┐ ┌──────────┐
│  Token   │ │ Est.     │ │ Est.     │
│  Usage   │ │ Cost     │ │ Cost     │
│  842K    │ │  $1.23   │ │  $34.56  │
│  24h     │ │  24h     │ │  30d     │
└──────────┘ └──────────┘ └──────────┘
```

**实现复杂度**：中。后端需要新字段 + 聚合查询；前端需要计算展示。

---

## 5. 延迟 Sparkline

**问题**：只看到账号"健康"还是"异常"，但不知道延迟趋势——是在变慢还是恢复中。

**方案**：账号列表里每个账号旁边显示一条微型折线图，展示最近 24h 的 P50 延迟。

**数据来源**：从 `request_logs` 按小时聚合 P50 延迟。

```
Accounts
┌──────────────────────────────────────────────────────┐
│ ● OpenAI Main    healthy   ▁▂▃▂▁▂▃▅▃▂▁▁  avg 1.2s  │
│ ● DeepSeek       healthy   ▁▁▂▁▁▂▁▂▁▁▁▁  avg 0.8s  │
│ ✕ OpenAI Backup  error     ▂▃▅▇█▇▅▃▅▇██  avg 5.1s  │
└──────────────────────────────────────────────────────┘
```

**实现复杂度**：中。后端需要新的聚合查询 API；前端需要简易 SVG sparkline 组件（无需引入图表库，手写 SVG path 即可）。

---

## 6. 一键测试连接

**问题**：添加账号后不确定 API key 是否有效、base_url 是否正确，要等下一轮健康检查才知道。

**方案**：账号创建/编辑表单里加一个 "Test Connection" 按钮，即时调用上游 `/models` 验证连通性。

**流程**：

```
用户点击 [Test Connection]
  → 前端调用 POST /admin/api/accounts/test
  → 后端用提供的 base_url + api_key 调 GET {base_url}/models
  → 返回结果：成功（列出可用模型数量）/ 失败（错误信息）
```

**UI**：

```
┌────────────────────────────────────────────┐
│  Add Account                               │
│                                            │
│  Label     [OpenAI Main           ]        │
│  Base URL  [https://api.openai.com/v1]     │
│  API Key   [sk-...                   ]     │
│                                            │
│  [Test Connection]                         │
│                                            │
│  ✓ Connected! 42 models available.         │
│    Response time: 320ms                    │
│                                            │
│  [Cancel]                      [Save]      │
└────────────────────────────────────────────┘
```

失败时：

```
│  ✕ Connection failed:                      │
│    401 Unauthorized - Invalid API key      │
```

**实现复杂度**：低。后端一个新端点（接收临时 base_url + api_key，不需要先保存到数据库）；前端一个按钮 + 状态展示。

---

## 优先级排序建议

| # | 功能 | 价值 | 复杂度 | 建议优先级 |
|---|---|---|---|---|
| 6 | 一键测试连接 | 高（配置流程中的即时反馈） | 低 | ★★★★★ |
| 1 | Provider 模板 | 高（减少手动输入错误） | 低 | ★★★★★ |
| 2 | 环境变量片段增强 | 中（v1 已有基础版） | 低 | ★★★★ |
| 3 | 内置 Playground | 高（端到端验证闭环） | 中 | ★★★★ |
| 4 | 成本估算 | 中（日常运营洞察） | 中 | ★★★ |
| 5 | 延迟 Sparkline | 低（锦上添花） | 中 | ★★ |

建议 v2 第一批做 #6 + #1（都是低复杂度高价值），第二批做 #3 + #4，#5 最后。
