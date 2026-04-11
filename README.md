# Lune

Lune 是一个面向个人自用的 **LLM API 网关**。

对下游暴露 OpenAI 兼容接口，通过内置后端引擎管理多个 LLM Provider（OpenAI、DeepSeek、Anthropic 等），提供统一的接入层。

**架构：** Client → Lune (Bearer Token 鉴权 + 路由调度) → Backend Engine → LLM Provider

---

## 功能

- 暴露 OpenAI 兼容接口：
  - `GET /v1/models`
  - `POST /openai/v1/chat/completions`
  - `POST /openai/v1/responses`
  - `POST /openai/v1/embeddings`
  - `POST /openai/v1/images/generations`
- Bearer Token 鉴权 + 调用额度管理
- 账号、号池、模型别名路由调度
- 请求日志 / 用量记录（SQLite）
- Web 控制台（嵌入式 React UI）
- Docker Compose 一键部署

## 不做什么

- 多租户 / 开放注册
- 企业权限体系
- TLS 指纹伪造 / 协议逆向 / 浏览器自动化

---

## 快速开始

### 1. 启动后端引擎

```bash
docker compose up -d backend
```

访问 `http://localhost:3000`，创建 Channel → 配置 LLM Provider → 生成 API Key。

### 2. 配置并启动 Lune

```bash
export LUNE_BACKEND_KEY=sk-xxxxx   # 后端引擎生成的 key
docker compose up -d lune
```

### 3. 使用

```bash
# 健康检查
curl http://localhost:7788/healthz

# 查看模型
curl -H "Authorization: Bearer sk-lune-local-dev" http://localhost:7788/v1/models

# 对话
curl -X POST http://localhost:7788/openai/v1/chat/completions \
  -H "Authorization: Bearer sk-lune-local-dev" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}'
```

控制台：`http://localhost:7788/admin`

---

## 配置

默认配置路径：`configs/config.json`（通过 `LUNE_CONFIG` 环境变量覆盖）

最小账号配置：

```json
{
  "accounts": [
    {
      "id": "backend-default",
      "platform": "backend",
      "label": "Lune Backend",
      "credential_type": "api_key",
      "credential_env": "LUNE_BACKEND_KEY",
      "plan_type": "plus",
      "enabled": true,
      "status": "healthy"
    }
  ]
}
```

完整配置示例见 `configs/config.example.json`。

---

## 开发

### Go 网关

```bash
go run ./cmd/lune                              # 开发运行
go test ./...                                  # 全部单元测试
go test -v ./internal/config                   # 单个包测试
CGO_ENABLED=0 go build -o lune ./cmd/lune      # 生产构建
```

### Web UI（React + Vite）

```bash
cd web
npm install
npm run build      # 类型检查 + vite 构建 → internal/site/dist/
npm run dev        # vite 开发服务器 :5173
```

---

## 数据

SQLite 数据库：`data/lune.db`

存储：请求日志、用量记录、访问令牌状态、账号状态。

---

## HTTP 端点

| 端点 | 说明 |
|------|------|
| `GET /healthz`, `GET /readyz` | 健康/就绪检查 |
| `GET /v1/models` | 可用模型列表 |
| `POST /openai/v1/chat/completions` | 聊天 API（需 Bearer Token） |
| `POST /openai/v1/responses` | Responses API |
| `POST /openai/v1/embeddings` | Embeddings API |
| `POST /openai/v1/images/generations` | 图像生成 API |
| `GET|POST /admin/api/*` | 管理 API（需 admin_token） |
| `/admin` | Web 控制台 |
