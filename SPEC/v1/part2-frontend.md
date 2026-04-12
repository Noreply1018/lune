# Lune v1 — Part 2: Frontend Design

> React + TypeScript admin UI. Embedded in the Go binary. No login wall.

## 1. Tech Stack

| Layer | Choice | Notes |
|---|---|---|
| Framework | React 18 | existing, no change |
| Language | TypeScript | existing, no change |
| Build | Vite | existing, no change |
| Styling | Tailwind CSS | existing, no change |
| Components | shadcn/ui | existing component library, no change |
| Routing | React Router | existing, no change |
| State | React hooks + fetch | no external state library needed |
| Embedding | `go:embed` via `internal/site/` | existing pipeline |

No new dependencies required. The frontend remains lightweight.

## 2. Auth Model Change

### Old model (delete):

```
User opens /admin → redirected to /login → enters admin token
→ token stored in sessionStorage → sent as Bearer on every API call
→ 401 triggers redirect to /login
```

### New model (v1):

```
User opens /admin → admin UI loads immediately
→ API calls to /admin/api/* use no auth header
→ the Go backend trusts localhost requests
```

Frontend auth changes:

- **Delete**: `lib/auth.ts` (sessionStorage-based token management)
- **Simplify**: `lib/api.ts` — plain fetch wrapper, no Bearer injection
- **Delete**: `LoginPage` component
- **Delete**: auth guard / redirect logic in router
- **Delete**: logout functionality

New `lib/api.ts`:

```typescript
const API_BASE = '/admin/api'

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : {},
    body: body ? JSON.stringify(body) : undefined,
  })

  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: { message: res.statusText } }))
    throw new ApiError(res.status, err.error?.message || 'Unknown error')
  }

  return res.json().then(r => r.data)
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body),
  put: <T>(path: string, body?: unknown) => request<T>('PUT', path, body),
  delete: <T>(path: string) => request<T>('DELETE', path),
}
```

## 3. Navigation Structure

### Sidebar

```
┌──────────────────┐
│  🌙 Lune         │
│                  │
│  ○ Overview      │
│  ○ Accounts      │
│  ○ Pools         │
│  ○ Routes        │
│  ○ Tokens        │
│  ○ Usage         │
│                  │
└──────────────────┘
```

6 pages total. No settings page (v1 settings are minimal and can be in overview or a settings section within overview).

### Routing

```typescript
const routes = [
  { path: '/admin',          element: <OverviewPage /> },
  { path: '/admin/accounts', element: <AccountsPage /> },
  { path: '/admin/pools',    element: <PoolsPage /> },
  { path: '/admin/routes',   element: <RoutesPage /> },
  { path: '/admin/tokens',   element: <TokensPage /> },
  { path: '/admin/usage',    element: <UsagePage /> },
]
```

No auth guards. All routes are directly accessible.

## 4. Page Designs

### 4.1 Overview Page (`/admin`)

The landing page. Shows system health at a glance.

```
┌─────────────────────────────────────────────────────────┐
│  Overview                                               │
│                                                         │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐   │
│  │ Accounts │ │  Pools   │ │  Tokens  │ │ Requests │   │
│  │  3 / 5   │ │    2     │ │    4     │ │  1,234   │   │
│  │ healthy  │ │ enabled  │ │ active   │ │  24h     │   │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘   │
│                                                         │
│  ┌──────────┐ ┌──────────┐                              │
│  │  Token   │ │ Success  │                              │
│  │  Usage   │ │  Rate    │                              │
│  │  842K    │ │  98.7%   │                              │
│  │  24h     │ │  24h     │                              │
│  └──────────┘ └──────────┘                              │
│                                                         │
│  Account Health                                         │
│  ┌──────────────────────────────────────────────────┐   │
│  │ ● OpenAI Main        healthy    checked 30s ago  │   │
│  │ ● DeepSeek Primary   healthy    checked 30s ago  │   │
│  │ ● OpenAI Backup      degraded   slow response    │   │
│  │ ○ Anthropic Test     disabled                    │   │
│  │ ✕ DeepSeek Backup    error      connection refused│  │
│  └──────────────────────────────────────────────────┘   │
│                                                         │
│  Recent Requests                                        │
│  ┌──────────────────────────────────────────────────┐   │
│  │ Time   Model    Token    Status  Latency  Tokens │   │
│  │ 10:01  gpt-4o   proj-a   200     1.2s     3,421  │   │
│  │ 10:00  gpt-4o   proj-b   200     0.8s     1,204  │   │
│  │ 09:58  ds-chat  proj-a   500     5.0s     -      │   │
│  └──────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

**Data source**: `GET /admin/api/overview`

**Refresh**: auto-poll every 10 seconds for health status updates.

### 4.2 Accounts Page (`/admin/accounts`)

List + CRUD for upstream accounts.

```
┌─────────────────────────────────────────────────────────┐
│  Accounts                              [+ Add Account]  │
│                                                         │
│  ┌──────────────────────────────────────────────────┐   │
│  │ Label           Base URL            Status Quota │   │
│  │                                                  │   │
│  │ ● OpenAI Main   api.openai.com/v1   healthy      │   │
│  │                                     $23 / $100   │   │
│  │                                                  │   │
│  │ ● DeepSeek      api.deepseek.com/v1 healthy      │   │
│  │                                     ¥5 / ¥50     │   │
│  │                                                  │   │
│  │ ○ Anthropic     api.anthropic.com/v1 disabled     │   │
│  │                                     - / -        │   │
│  └──────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

**Status badge colors**:
- `healthy` → green
- `degraded` → yellow
- `error` → red
- `disabled` → gray

**Row actions** (dropdown or inline):
- Edit
- Enable / Disable
- Delete (with confirmation)

**Add/Edit dialog**:

```
┌────────────────────────────────────────────┐
│  Add Account                               │
│                                            │
│  Label         [OpenAI Main           ]    │
│  Base URL      [https://api.openai.com/v1] │
│  API Key       [sk-•••••••••••••••••• ]    │
│                                            │
│  Model Allowlist (optional)                │
│  [gpt-4o, gpt-4o-mini, gpt-4.1      ]     │
│  (comma-separated, empty = all models)     │
│                                            │
│  Quota                                     │
│  Total  [100    ]  Unit [USD ▼]            │
│  Used   [23     ]                          │
│                                            │
│  Notes                                     │
│  [Personal account, $20/month plan   ]     │
│                                            │
│  [Cancel]                      [Save]      │
└────────────────────────────────────────────┘
```

**Important**: API key field shows `••••••••` when editing an existing account. If the user leaves it unchanged, the PUT request omits the field (don't overwrite with empty). Only send `api_key` if the user explicitly types a new value.

### 4.3 Pools Page (`/admin/pools`)

Pool management with inline member editing.

```
┌─────────────────────────────────────────────────────────┐
│  Pools                                    [+ Add Pool]  │
│                                                         │
│  ┌──────────────────────────────────────────────────┐   │
│  │ ▼ OpenAI Pool          priority-first-healthy    │   │
│  │   Enabled ●                                      │   │
│  │                                                  │   │
│  │   Members:                                       │   │
│  │   ┌───────────────────────────────────────────┐  │   │
│  │   │ Priority  Account         Weight  Status  │  │   │
│  │   │ 1         OpenAI Main     10      ●       │  │   │
│  │   │ 2         OpenAI Backup   5       ●       │  │   │
│  │   └───────────────────────────────────────────┘  │   │
│  │   [+ Add Member]    [Edit]    [Disable]          │   │
│  │                                                  │   │
│  │ ▶ DeepSeek Pool     priority-first-healthy       │   │
│  │   Enabled ●                                      │   │
│  └──────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

Pools are shown as expandable cards. Collapsed shows pool name, strategy, enabled state. Expanded shows member list.

**Add member**: select from existing accounts (dropdown), set priority and weight.

**Member editing**: inline number inputs for priority and weight. Changes save on blur or enter.

### 4.4 Routes Page (`/admin/routes`)

Model route configuration.

```
┌─────────────────────────────────────────────────────────┐
│  Routes                                  [+ Add Route]  │
│                                                         │
│  Default Pool: [OpenAI Pool ▼]  (catch-all for          │
│                                  unmatched models)      │
│                                                         │
│  ┌──────────────────────────────────────────────────┐   │
│  │ Alias           Target Model    Pool       On/Off│   │
│  │ gpt-4o          gpt-4o          OpenAI Pool  ●   │   │
│  │ gpt-4o-mini     gpt-4o-mini     OpenAI Pool  ●   │   │
│  │ deepseek-chat   deepseek-chat   DeepSeek     ●   │   │
│  │ claude-sonnet   claude-sonnet   Anthropic    ○   │   │
│  └──────────────────────────────────────────────────┘   │
│                                                         │
│  ℹ Models not listed above will route through the       │
│    default pool with the original model name.           │
└─────────────────────────────────────────────────────────┘
```

**Default pool selector**: at the top of the page. Dropdown of enabled pools + "None" option. Saves to `system_config.default_pool_id`.

**Add/Edit dialog**:

```
┌────────────────────────────────────────────┐
│  Add Route                                 │
│                                            │
│  Alias          [gpt-4o              ]     │
│  Target Model   [gpt-4o              ]     │
│  Pool           [OpenAI Pool ▼       ]     │
│  Enabled        [✓]                        │
│                                            │
│  [Cancel]                      [Save]      │
└────────────────────────────────────────────┘
```

If alias = target model (most common case), a helper hint: "Alias and target are the same — model name is passed through unchanged."

### 4.5 Tokens Page (`/admin/tokens`)

Access token management.

```
┌─────────────────────────────────────────────────────────┐
│  Tokens                                 [+ Add Token]   │
│                                                         │
│  ┌──────────────────────────────────────────────────┐   │
│  │ Name       Token            Usage       Last Used│   │
│  │                                                  │   │
│  │ proj-main  sk-lune-•••a3f2  823K / 1M   2 min   │   │
│  │            [Copy]           ████████░░  ago      │   │
│  │                                                  │   │
│  │ proj-test  sk-lune-•••d891  12K / ∞     1 hour  │   │
│  │            [Copy]                       ago      │   │
│  │                                                  │   │
│  │ disabled   sk-lune-•••f102  0 / 100K    never    │   │
│  │            [Copy]           (disabled)           │   │
│  └──────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

**Key UX elements**:

- **Copy button**: one-click copy token to clipboard. Show brief "Copied!" toast.
- **Usage bar**: visual progress bar for `used_tokens / quota_tokens`. Color: green (<70%), yellow (70-90%), red (>90%).
- **Unlimited**: when `quota_tokens = 0`, show "∞" and no progress bar.
- **Masked token**: show `sk-lune-•••{last4}`. Full token only shown once at creation.

**Add dialog**:

```
┌────────────────────────────────────────────┐
│  Create Access Token                       │
│                                            │
│  Name           [my-project          ]     │
│                                            │
│  Token (auto-generated if empty)           │
│  [                               ]         │
│                                            │
│  Token Quota                               │
│  [1000000   ] tokens (0 = unlimited)       │
│                                            │
│  [Cancel]                      [Create]    │
└────────────────────────────────────────────┘
```

**After creation**: show a one-time dialog with the full token value and a prominent copy button:

```
┌────────────────────────────────────────────┐
│  ✓ Token Created                           │
│                                            │
│  Make sure to copy your token now.         │
│  You won't be able to see it again.        │
│                                            │
│  sk-lune-a8f2c1d9e3b7...4f2a               │
│                                            │
│  [Copy Token]                              │
│                                            │
│  Quick Setup:                              │
│  ┌──────────────────────────────────────┐  │
│  │ export OPENAI_BASE_URL=              │  │
│  │   http://127.0.0.1:7788/v1          │  │
│  │ export OPENAI_API_KEY=              │  │
│  │   sk-lune-a8f2c1d9e3b7...4f2a      │  │
│  └──────────────────────────────────────┘  │
│  [Copy Env Vars]                           │
│                                            │
│  [Done]                                    │
└────────────────────────────────────────────┘
```

注意：这里的 "Quick Setup" 代码块和 "Copy Env Vars" 按钮是 v1 便利功能——用户创建 token 后可以一键复制完整的环境变量配置，直接粘贴到项目的 `.env` 文件里。

### 4.6 Usage Page (`/admin/usage`)

Request logs and usage breakdown.

```
┌─────────────────────────────────────────────────────────┐
│  Usage                                                  │
│                                                         │
│  Time Range: [Last 24h ▼]  Token: [All ▼]              │
│  Account:    [All ▼]       Model: [All ▼]              │
│                                                         │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐                │
│  │ Requests │ │  Input   │ │  Output  │                │
│  │  1,234   │ │  Tokens  │ │  Tokens  │                │
│  │          │ │  456K    │ │  234K    │                │
│  └──────────┘ └──────────┘ └──────────┘                │
│                                                         │
│  Usage by Account                                       │
│  ┌──────────────────────────────────────────────────┐   │
│  │ Account         Requests  Input    Output  Total │   │
│  │ OpenAI Main     800       300K     150K    450K  │   │
│  │ DeepSeek        400       150K     80K     230K  │   │
│  │ OpenAI Backup   34        6K       4K      10K   │   │
│  └──────────────────────────────────────────────────┘   │
│                                                         │
│  Usage by Token                                         │
│  ┌──────────────────────────────────────────────────┐   │
│  │ Token       Requests  Input    Output  Total     │   │
│  │ proj-main   900       400K     200K    600K      │   │
│  │ proj-test   334       56K      34K     90K       │   │
│  └──────────────────────────────────────────────────┘   │
│                                                         │
│  Request Log                                            │
│  ┌──────────────────────────────────────────────────┐   │
│  │ Time   Model     Token     Account   Status      │   │
│  │        Alias     Name      Label     In/Out      │   │
│  │                                                  │   │
│  │ 10:01  gpt-4o    proj-a    OpenAI    200         │   │
│  │                            Main      1.2K/2.2K   │   │
│  │ 10:00  gpt-4o    proj-b    OpenAI    200         │   │
│  │                            Main      0.5K/0.7K   │   │
│  │ 09:58  ds-chat   proj-a    DeepSeek  500         │   │
│  │                                      error       │   │
│  └──────────────────────────────────────────────────┘   │
│                                                         │
│  [< Prev]  Page 1 of 12  [Next >]                      │
└─────────────────────────────────────────────────────────┘
```

**Filters**: time range, token, account, model. All are dropdowns populated from the data.

**Pagination**: 50 rows per page for request log.

**Time ranges**: Last 1h, Last 24h, Last 7d, Last 30d, All time.

## 5. Shared Components

### 5.1 StatusBadge

Reusable health status indicator.

```typescript
type Status = 'healthy' | 'degraded' | 'error' | 'disabled'

const colors: Record<Status, string> = {
  healthy:  'bg-green-500',
  degraded: 'bg-yellow-500',
  error:    'bg-red-500',
  disabled: 'bg-gray-400',
}
```

Renders as a colored dot + text label.

### 5.2 CopyButton

One-click copy to clipboard with toast feedback.

```typescript
function CopyButton({ value, label = 'Copy' }: { value: string; label?: string }) {
  const [copied, setCopied] = useState(false)
  // ...
}
```

Shows "Copied!" for 2 seconds after click.

### 5.3 StatCard

Dashboard stat display (used on Overview page).

```typescript
function StatCard({ title, value, subtitle }: {
  title: string
  value: string | number
  subtitle?: string
}) {
  // ...
}
```

### 5.4 DataTable

Reusable table with sorting. Uses the existing shadcn Table component underneath.

### 5.5 ConfirmDialog

Confirmation dialog for destructive actions (delete account, delete token, etc.).

### 5.6 UsageBar

Visual progress bar for token quota usage.

```typescript
function UsageBar({ used, total }: { used: number; total: number }) {
  // total = 0 means unlimited, don't show bar
  if (total === 0) return <span className="text-muted-foreground">∞</span>
  const pct = Math.min(100, (used / total) * 100)
  const color = pct > 90 ? 'bg-red-500' : pct > 70 ? 'bg-yellow-500' : 'bg-green-500'
  // ...
}
```

## 6. Data Fetching Patterns

### 6.1 Simple fetch + state

No external state management library. Each page manages its own data:

```typescript
function AccountsPage() {
  const [accounts, setAccounts] = useState<Account[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.get<Account[]>('/accounts').then(setAccounts).finally(() => setLoading(false))
  }, [])

  // ...
}
```

### 6.2 Polling for real-time data

Overview page and Accounts page poll for health updates:

```typescript
useEffect(() => {
  const interval = setInterval(() => {
    api.get<Account[]>('/accounts').then(setAccounts)
  }, 10_000)  // every 10 seconds
  return () => clearInterval(interval)
}, [])
```

### 6.3 Optimistic updates

For toggle operations (enable/disable), update local state immediately, then fire the API call:

```typescript
async function toggleAccount(id: number, enabled: boolean) {
  // Optimistic update
  setAccounts(prev => prev.map(a => a.id === id ? { ...a, enabled } : a))
  // API call
  try {
    await api.post(`/accounts/${id}/${enabled ? 'enable' : 'disable'}`)
  } catch {
    // Revert on error
    setAccounts(prev => prev.map(a => a.id === id ? { ...a, enabled: !enabled } : a))
    toast.error('Failed to update account')
  }
}
```

## 7. Responsive Design

v1 is optimized for desktop use (personal tool, used on the developer's machine). However:

- sidebar collapses to icons on narrow screens
- tables scroll horizontally on small viewports
- dialogs are max-width constrained

Minimum supported width: 1024px. Below that, basic usability but no optimization effort.

## 8. Theme

Keep the existing Tailwind + shadcn/ui theme. Dark mode support if already configured in the existing setup; otherwise, use system default.

No custom branding beyond the "Lune" name and a minimal logo/icon.

## 9. Frontend Implementation Phases

### Phase 1 (with backend Phase 1)

- Shell / layout with new navigation
- API client (no auth)
- Accounts page (list, create, edit, enable/disable, delete)
- Pools page (list, create, edit members, enable/disable, delete)
- Routes page (list, create, edit, delete, default pool selector)
- Tokens page (list, create, copy, enable/disable, delete)

### Phase 2 (with backend Phase 2)

- StatusBadge component with real health data
- Account health indicators (polling)
- Token creation dialog with env var snippet
- UsageBar component

### Phase 3 (with backend Phase 3)

- Overview page (stats, health summary, recent requests)
- Usage page (filters, breakdowns, paginated logs)
- Polish: loading states, error handling, empty states, toasts

## 10. Type Definitions

Core TypeScript types matching the backend domain model:

```typescript
interface Account {
  id: number
  label: string
  base_url: string
  api_key_set: boolean     // never expose actual key, just whether it's set
  api_key_masked: string   // "sk-...xxxx"
  enabled: boolean
  status: 'healthy' | 'degraded' | 'error' | 'disabled'
  quota_total: number
  quota_used: number
  quota_unit: string
  notes: string
  model_allowlist: string[]
  last_checked_at: string | null
  last_error: string | null
  created_at: string
  updated_at: string
}

interface Pool {
  id: number
  label: string
  strategy: string
  enabled: boolean
  members: PoolMember[]
  created_at: string
  updated_at: string
}

interface PoolMember {
  id: number
  account_id: number
  account_label: string    // denormalized for display
  account_status: string   // denormalized for display
  priority: number
  weight: number
}

interface ModelRoute {
  id: number
  alias: string
  pool_id: number
  pool_label: string       // denormalized for display
  target_model: string
  enabled: boolean
  created_at: string
  updated_at: string
}

interface AccessToken {
  id: number
  name: string
  token_masked: string     // "sk-lune-•••xxxx"
  enabled: boolean
  quota_tokens: number     // 0 = unlimited
  used_tokens: number
  created_at: string
  updated_at: string
  last_used_at: string | null
}

// Only returned on creation
interface AccessTokenCreated {
  id: number
  name: string
  token: string            // full token value, shown only once
  quota_tokens: number
}

interface RequestLog {
  id: number
  request_id: string
  access_token_name: string
  model_alias: string
  target_model: string
  pool_id: number
  account_id: number
  account_label: string    // denormalized
  status_code: number
  latency_ms: number
  input_tokens: number | null
  output_tokens: number | null
  stream: boolean
  request_ip: string
  success: boolean
  error_message: string | null
  created_at: string
}

interface Overview {
  total_accounts: number
  healthy_accounts: number
  total_pools: number
  total_tokens: number
  requests_24h: number
  success_rate_24h: number
  token_usage_24h: {
    input: number
    output: number
  }
  account_health: Array<{
    id: number
    label: string
    status: string
    last_checked_at: string | null
    last_error: string | null
  }>
  recent_requests: RequestLog[]
}

interface UsageStats {
  total_requests: number
  total_input_tokens: number
  total_output_tokens: number
  by_account: Array<{
    account_id: number
    account_label: string
    requests: number
    input_tokens: number
    output_tokens: number
  }>
  by_token: Array<{
    token_name: string
    requests: number
    input_tokens: number
    output_tokens: number
  }>
  logs: {
    items: RequestLog[]
    total: number
    page: number
    page_size: number
  }
}

interface SystemSettings {
  admin_token_masked: string
  default_pool_id: number | null
  health_check_interval: number
  request_timeout: number
  max_retry_attempts: number
}
```
