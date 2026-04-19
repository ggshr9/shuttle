# Shuttle GUI 重构设计（full overhaul）

- **Date**: 2026-04-19
- **Scope**: `gui/web/src/` 完整重写（架构 + 设计系统 + UX）
- **Strategy**: 单分支分阶段 commit，每阶段可独立合入 `main`
- **Visual direction**: Geist-inspired（A 方向，确认于 brainstorm）

---

## 1 · 目标与分层约束

### 核心目标
1. **架构清晰** — 每个目录职责单一，依赖方向单向
2. **实现优雅** — 每个文件可独立阅读、理解成本低、代码量贴合功能体量
3. **维护简单** — 添加/删除一个功能是局部操作；新人 30 分钟读懂主干

### 分层依赖

上层可依赖下层，反之不可：

```
┌──────────────────────────────────────────────────┐
│ features/*    功能切片（业务视图 + 状态 + 类型）  │
├──────────────────────────────────────────────────┤
│ app/          应用外壳（shell / sidebar / routes）│
├──────────────────────────────────────────────────┤
│ ui/           设计系统 primitive（纯展示，无业务）│
├──────────────────────────────────────────────────┤
│ lib/          跨域基础设施（api/resource/router…）│
└──────────────────────────────────────────────────┘
```

### 硬性约束
- `ui/` 禁止 import `features/`、`app/`、`lib/api/*`
- `features/<x>/` 禁止 import `features/<y>/`；共享代码下沉到 `ui/` 或 `lib/`
- `lib/` 是纯工具，禁止引用 Svelte 组件
- `app/` 只做布局和路由装配，不含业务逻辑
- 所有数据获取走 `lib/resource` + `lib/api`，禁止 page 内 `fetch` 直调
- 所有 UI primitive 走 `ui/`，禁止 page 内重写 Card/Button/Modal

### 非目标
- 不引入 Tailwind
- 不引入 TanStack Query（注：`@tanstack/virtual-core` 是无关的虚拟滚动工具，可在 Logs 引入）
- 不替换 i18n 系统（仅重组字符串文件）
- 不改后端 API（GUI 层纯消费）

---

## 2 · 目录结构

```
gui/web/src/
│
├─ main.ts                       ← 挂 App，注册全局样式
├─ app.css                       ← CSS reset + tokens 引入
│
├─ app/                          ← 应用外壳（5 文件）
│   ├─ App.svelte                ← ~80 行：router 挂载 + 全局 Toast
│   ├─ Shell.svelte              ← sidebar + content 布局
│   ├─ Sidebar.svelte            ← 侧边栏导航
│   ├─ routes.ts                 ← 路由表（feature.route 装配）
│   └─ icons.ts                  ← icon map（替代 inline SVG）
│
├─ ui/                           ← 设计系统 primitive（~18 文件）
│   ├─ tokens.css                ← Geist 设计 token（颜色/字号/间距/radius/shadow）
│   ├─ index.ts                  ← 聚合出口
│   ├─ Button.svelte             ← variant: primary/secondary/ghost/danger
│   ├─ Card.svelte
│   ├─ Input.svelte
│   ├─ Select.svelte             ← bits-ui 封装
│   ├─ Switch.svelte             ← bits-ui 封装
│   ├─ Dialog.svelte             ← bits-ui 封装
│   ├─ DropdownMenu.svelte       ← bits-ui 封装
│   ├─ Tabs.svelte               ← bits-ui 封装
│   ├─ Tooltip.svelte            ← bits-ui 封装
│   ├─ Combobox.svelte           ← bits-ui 封装
│   ├─ Badge.svelte
│   ├─ Icon.svelte
│   ├─ StatRow.svelte
│   ├─ Empty.svelte
│   ├─ Spinner.svelte
│   ├─ ErrorBanner.svelte
│   ├─ Section.svelte
│   └─ AsyncBoundary.svelte      ← 配合 Resource 统一处理 loading/error/empty
│
├─ lib/                          ← 跨域基础设施（~15 文件）
│   ├─ api/
│   │   ├─ client.ts             ← createClient({ base, version, auth }) 工厂
│   │   ├─ types.ts              ← 所有后端 type
│   │   └─ endpoints.ts          ← 按 feature 分组的 API 函数
│   ├─ ws.ts
│   ├─ resource.svelte.ts        ← createResource / createStream / invalidate
│   ├─ router/
│   │   ├─ Router.svelte
│   │   ├─ Route.svelte
│   │   ├─ Link.svelte
│   │   └─ router.svelte.ts      ← navigate / useRoute / useParams
│   ├─ i18n/
│   │   ├─ index.ts              ← 现有 API 不变
│   │   └─ strings/              ← en.ts / zh.ts
│   ├─ theme.svelte.ts
│   ├─ toast.svelte.ts
│   ├─ shortcuts.ts
│   ├─ notify.ts
│   └─ flags.ts                  ← 预留（非本次实现）
│
└─ features/                     ← 9 个功能切片
    ├─ dashboard/
    │   ├─ index.ts              ← 导出 route + public API
    │   ├─ Dashboard.svelte
    │   ├─ ConnectionHero.svelte
    │   ├─ StatsGrid.svelte
    │   ├─ SpeedSparkline.svelte
    │   ├─ TransportBreakdown.svelte
    │   ├─ ExpandedPanel.svelte
    │   └─ resource.ts
    ├─ servers/
    │   ├─ index.ts
    │   ├─ ServersPage.svelte
    │   ├─ ServerTable.svelte
    │   ├─ ServerRowExpanded.svelte
    │   ├─ AddServerDialog.svelte
    │   ├─ ImportDialog.svelte
    │   ├─ DeleteConfirm.svelte
    │   └─ resource.ts
    ├─ subscriptions/
    ├─ groups/
    ├─ routing/
    ├─ mesh/
    ├─ logs/
    ├─ settings/
    │   ├─ index.ts
    │   ├─ SettingsLayout.svelte
    │   ├─ pages/
    │   │   ├─ proxy/
    │   │   ├─ mesh/
    │   │   ├─ dns/
    │   │   ├─ qos/
    │   │   ├─ geodata/
    │   │   ├─ appearance/
    │   │   ├─ update/
    │   │   ├─ backup/
    │   │   ├─ diagnostics/
    │   │   └─ logs/
    │   └─ resource.ts
    └─ onboarding/
        ├─ index.ts
        ├─ OnboardingFlow.svelte
        ├─ steps/
        │   ├─ WelcomeStep.svelte
        │   ├─ ImportStep.svelte
        │   ├─ TestStep.svelte
        │   └─ DoneStep.svelte
        └─ resource.ts
```

---

## 3 · 可扩展性约定

### ① Feature self-description
每个 `features/<x>/index.ts` 是唯一对外出口：

```ts
// features/servers/index.ts
import { lazy } from '../../lib/router'

export const route = {
  path: '/servers',
  component: lazy(() => import('./ServersPage.svelte')),
  nav: { label: 'nav.servers', icon: 'servers', order: 20 },
}

export { useServers, useActiveServer } from './resource'
```

`app/routes.ts` 只装配：

```ts
import * as dashboard from '../features/dashboard'
import * as servers from '../features/servers'
// ...
export const routes = [dashboard.route, servers.route, /* ... */]
  .sort((a, b) => (a.nav?.order ?? 999) - (b.nav?.order ?? 999))
```

**加一个 feature = 新建目录 + `routes.ts` 加一行 import。不触任何其他文件。**

### ② Feature 私有性
feature 内除 `index.ts` 导出内容外，均为私有。跨 feature 需要共享的代码下沉到 `ui/` 或 `lib/`。

同样适用 `ui/` —— 统一 `import { Button, Card } from '@/ui'`。

### ③ Settings 子路由自描述
每个 settings 子页自己声明 sub-nav：

```ts
// features/settings/pages/mesh/index.ts
export const subRoute = {
  path: 'mesh',
  component: Mesh,
  nav: { label: 'settings.mesh', icon: 'mesh' },
}
```

`SettingsLayout.svelte` 从目录汇总。

### ④ Token 命名空间
CSS 变量全部 `--shuttle-<scope>-<prop>` 前缀。Wails WebView 嵌入无冲突；未来拆独立 npm 包可直接带走。

### ⑤ Resource 统一契约
所有 `createResource` 返回 `{ data, loading, error, stale, refetch }`。

### ⑥ API client 版本化
`lib/api/client.ts` 写成 `createClient({ base, version, auth })` 工厂。

### ⑦ 贡献者文档
写 `gui/web/src/README.md`（~100 行）：分层规则 + "如何加 feature" + "如何加 primitive"。

### ⑧ Feature flags hook 位
`lib/flags.ts` 预留位置，不提前实现。

---

## 4 · Design System

### 4.1 Token 体系（`ui/tokens.css`）

```
颜色（语义命名，light/dark 各一套）
  --shuttle-bg-base           页面底
  --shuttle-bg-surface        卡片底
  --shuttle-bg-subtle         hover / 分组底
  --shuttle-fg-primary        主文字
  --shuttle-fg-secondary      次文字
  --shuttle-fg-muted          占位
  --shuttle-border            分割线
  --shuttle-border-strong     hover / focus 线
  --shuttle-accent            唯一强调色（inverse 按钮底）
  --shuttle-accent-fg         accent 上的文字
  --shuttle-success / warning / danger / info    仅状态场景使用

排版
  --shuttle-font-sans    Inter, system
  --shuttle-font-mono    SF Mono, ui-monospace
  --shuttle-text-xs (11) / sm (12) / base (14) / lg (16) / xl (20) / 2xl (28)
  --shuttle-weight-regular (400) / medium (500) / semibold (600)
  --shuttle-tracking-tight (-0.02em) / normal (0)

间距（4px 网格）
  --shuttle-space-0..8    0 / 4 / 8 / 12 / 16 / 24 / 32 / 48

圆角
  --shuttle-radius-sm / md / lg    4 / 8 / 12

阴影（极轻）
  --shuttle-shadow-sm / md

动效
  --shuttle-duration    120ms
  --shuttle-easing      cubic-bezier(0.2, 0, 0, 1)
```

**具体色值（Geist-inspired）**

Dark（默认）：
- bg-base `#0a0a0a`，bg-surface `#111113`，bg-subtle `#1a1a1c`
- fg-primary `#ededed`，fg-secondary `#a1a1aa`，fg-muted `#52525b`
- border `#27272a`，border-strong `#3f3f46`
- accent `#ededed`（inverse），accent-fg `#09090b`
- success `#22c55e`，warning `#eab308`，danger `#ef4444`

Light：
- bg-base `#fafafa`，bg-surface `#ffffff`，bg-subtle `#f4f4f5`
- fg-primary `#09090b`，fg-secondary `#52525b`，fg-muted `#a1a1aa`
- border `#e4e4e7`，border-strong `#d4d4d8`
- accent `#09090b`（inverse），accent-fg `#fafafa`
- success `#16a34a`，warning `#ca8a04`，danger `#dc2626`

### 4.2 视觉语言三条硬规则
1. **单一强调色** —— accent 唯一；状态色仅用于表示状态
2. **无渐变 / 无 glow / 无 glassmorphism** —— 深度靠 1px border + 极轻 shadow
3. **字号即层级** —— 不靠粗体或颜色堆层级

### 4.3 Primitive 契约
- 纯展示：禁止 import `lib/api` 或 feature 内部
- 一致 props：`size: 'sm'|'md'`、`variant: 'primary'|'secondary'|'ghost'|'danger'`、`loading`、`disabled`、`onclick`
- Slot 统一：`children`、可选 `header` / `footer` / `actions`
- 无内部副作用：不存 localStorage、不发请求、不触发全局状态
- a11y 默认开启：focus ring、aria、键盘导航（bits-ui 保障）

---

## 5 · 数据层（Resource）

### 5.1 `createResource` 契约

```ts
interface Resource<T> {
  readonly data: T | undefined
  readonly loading: boolean
  readonly error: Error | null
  readonly stale: boolean
  refetch(): Promise<void>
}

interface Options<T> {
  poll?: number                  // ms，0/undefined 不轮询
  initial?: T
  enabled?: () => boolean        // 动态启停
  onError?: (e: Error) => void
}

function createResource<T>(
  key: string,                   // 单例键，同 key 全局共享
  fetcher: () => Promise<T>,
  opts?: Options<T>,
): Resource<T>

function invalidate(key: string): void
function invalidateAll(): void
```

### 5.2 使用模式

```ts
// features/dashboard/resource.ts
export const useStatus = () => createResource(
  'dashboard.status',
  api.getStatus,
  { poll: 3000 },
)

export const useTransportStats = () => createResource(
  'dashboard.transport-stats',
  api.getTransportStats,
  { poll: 5000, enabled: () => useStatus().data?.state === 'running' },
)
```

### 5.3 设计要点
- **单例按 key 共享** —— 多处订阅同 key 只跑一个 fetch / interval
- **自动生命周期** —— 订阅归零暂停，新订阅恢复
- **细粒度 reactive** —— 返回对象用 runes `$state`，字段级 tracking
- **可暂停** —— `enabled` 支持按外部状态启停
- **失败不阻塞** —— 错误写 `error`，`data` 保留为上次成功值
- **手动 invalidate** —— mutation 成功后调用

### 5.4 Mutation 约定
不抽象，直接在 feature 的 `resource.ts` 导出异步函数：

```ts
export async function addServer(srv: Server) {
  await api.addServer(srv)
  invalidate('servers.list')
}
```

Page 直接 import 使用；try/catch 由 page 处理（配 toast）。

### 5.5 WebSocket 流
`createStream` 工厂：`{ data, connected, close() }`。用法与 resource 对称但无 refetch。

---

## 6 · Router

### 6.1 路由契约

```ts
interface RouteDef {
  path: string                   // '/', '/servers', '/settings/:section'
  component: Component | (() => Promise<Component>)
  nav?: { label: string; icon: string; order: number; hidden?: boolean }
  children?: RouteDef[]
}

// 全局状态
const route = {
  path: $state<string>('/'),
  params: $derived<Record<string, string>>({...}),
  query: $derived<Record<string, string>>({...}),
}

function navigate(path: string, opts?: { replace?: boolean }): void
function useRoute(): typeof route
function useParams<T>(): T
function matches(pattern: string): boolean
function lazy<T>(loader: () => Promise<{ default: T }>): () => Promise<T>
```

### 6.2 URL 表

| Hash | Feature |
|------|---------|
| `#/` | Dashboard |
| `#/servers` | Servers |
| `#/subscriptions` | Subscriptions |
| `#/groups` | Groups |
| `#/groups/:id` | Group detail |
| `#/routing` | Routing |
| `#/mesh` | Mesh |
| `#/logs` | Logs |
| `#/settings` | 重定向到 `#/settings/proxy` |
| `#/settings/:section` | Settings sub-page |
| `#/onboarding` | 覆盖式，绕过 Shell |

### 6.3 不做
- 路由 guard（无登录态；Onboarding 由条件渲染）
- transition 动画（保持快速切换）
- scroll 恢复（每页自管）

### 6.4 深链与键盘
- Wails URL handler 解析 `shuttle://goto?path=/settings/mesh` → `navigate()`
- `Cmd+,` → `/settings`；`Cmd+1..9` → 顶级 tab
- 未匹配路径 → 重定向 `/`（不 404）

---

## 7 · Feature 页面重设计

### 7.1 Dashboard（详细设计，参考 §设计图）
- 删 `SimpleMode.svelte`
- 纵向滚动 = 渐进披露（小窗口只见 Hero = 原 SimpleMode 全部信息）
- Hero 卡：状态点 + 服务器名 + 两列速度 + 主操作三列水平排
- Stats grid（4-up）：RTT / Loss / Transfer / Transport
- Throughput chart（细小 sparkline，5 min）
- Active transports 密集表

### 7.2 Servers
- 密集表（48px/行，列：状态 / 名称 / 地址 / 延迟 / 协议 / 操作）
- 点击行内嵌展开（密码 / 测速历史 / 手动切换）
- 多选 + 批量操作（删除 / 导出 / 测速）
- active 行 `border-left: 2px var(--accent)`
- `+ Add server` 按钮打开 bits-ui Dialog

### 7.3 Subscriptions
- 表 + 行展开（同 Servers 模式）
- 列：源 / 协议类型 / 服务器数 / 上次更新 / 自动刷新
- 手动刷新按钮 hover 显形

### 7.4 Groups
- 4 列卡片网格（信息量大）
- 卡展示：组名 / 策略 / 成员数 / 当前活跃服务器 / 近 24h 质量曲线
- 点击进 `#/groups/:id`
- 末尾 dashed "+ New group" 占位卡

### 7.5 Routing
- **保留整体结构**，换组件层
- 新增：规则命中可视化 bar（顶部 10px 分段，hover 显示命中次数）

### 7.6 Mesh
- 上半：拓扑图保留
- 下半：对等节点密集表（与 Servers 风格统一）
- 删除多余说明 block（移到 Tooltip）

### 7.7 Logs
- 三栏：左 filter / 中 list / 右 detail
- 等宽字体 + level 着色（克制饱和度）
- 顶部固定搜索
- 虚拟滚动（引入 `@tanstack/virtual-core` 或自写）

### 7.8 Settings
- **左侧 sub-nav（不是 tab）**
- 每子页独立 URL
- 页面内仅该主题内容，不超一屏
- 所有开关 `ui/Switch`、下拉 `ui/Select`
- "unsaved changes" bar（顶部）：`[Discard] [Save]`

### 7.9 Onboarding
- 全屏覆盖式 4 步 wizard
- 每步结构：大标题 + 说明 + 单列操作 + 底部 `[Back] [Next]`
- 点状进度指示（● ● ○ ○），非 progress bar

### 7.10 通用 UX 准则
1. 空态 / 加载态 / 错误态三件套，由 `<AsyncBoundary resource={...}>` 自动装配
2. Toast 只用于异步结果；表单错误贴字段旁
3. Dialog 只用于决策交互；详情用 in-page expand 或侧抽屉
4. 键盘可达：每 page 至少支持 `↑↓ / Enter / Esc`
5. 单位永远显式
6. 颜色只用于语义（绿=好、红=坏、黄=警），装饰用中性灰阶

---

## 8 · 交付阶段

每阶段一个 PR，自成 green，可独立合入 `main`。

| # | PR | 范围 | 依赖 | 估 |
|---|---|------|------|---|
| **P1** | `ui + lib 基础设施` | `ui/tokens.css` + 18 primitive（含 6 个 bits-ui 封装）；`lib/api` 拆分；`lib/resource`；`lib/router`；`lib/theme/toast` runes 化；装 bits-ui；`/__ui__` 开发 harness | — | 2-3 天 |
| **P2** | `app shell 切换` | `app/App` + `Shell` + `Sidebar` + `routes.ts`；路由挂 `/`；各 tab 临时 bridge 到旧 `pages/*.svelte`；删除 `SimpleMode.svelte`；旧 CSS token 移到 `ui/tokens.css` | P1 | 1-2 天 |
| **P3** | `feature: dashboard` | `features/dashboard/` 完整 §7.1；删 `pages/Dashboard.svelte`、`pages/SimpleMode.svelte`；`SpeedChart` / `TrafficChart` / `ConnectionQualityChart` / `SpeedTestHistory` 从 `lib/` 迁入并统一为 `features/dashboard/` 内的组件 | P2 | 2-3 天 |
| **P4** | `feature: servers` | 密集表 / inline expand / 多选 / 批量 | P2 | 2-3 天 |
| **P5** | `feature: subscriptions + groups` | bundle，含 `/groups/:id` 子路由 | P2 | 2-3 天 |
| **P6** | `feature: routing` | 业务保留，换组件 + 命中可视化 | P2 | 2 天 |
| **P7** | `feature: mesh` | 拓扑保留 + 表格重做 | P2 | 1-2 天 |
| **P8** | `feature: logs` | 三栏 + 虚拟滚动 | P2 | 2 天 |
| **P9** | `feature: settings` | 子路由 + sub-nav + 10 子页 + unsaved bar | P2 | 2-3 天 |
| **P10** | `onboarding + 清理` | Onboarding 重写；删除 `pages/*`、`lib/Onboarding.svelte`、bridge、旧 token；`src/` 最终只剩 `app / ui / lib / features / main.ts / app.css` | P3-P9 | 1-2 天 |
| **P11** | `测试 + 质量` | Playwright E2E 更新 + a11y（axe）+ visual regression（可选）+ CHANGELOG + `src/README.md` | P10 | 1-2 天 |

**总估算**：18-25 工作日。

**并行性**：P3-P9 两两独立（都只依赖 P2），可并行；建议仍串行以聚焦 review。

### 每个 PR 的自检清单
- [ ] `svelte-check` 0 error
- [ ] `npm run build` 成功
- [ ] bundle size 不退化（P1 记基准）
- [ ] 未动的 feature 手工 smoke（防 bridge 破坏）
- [ ] 对应 Playwright E2E 全绿

### 可取消性
任何阶段完成后可 pause；`main` 始终可用；已完成的 feature 先上线。

---

## 9 · 测试策略

| 层 | 工具 | 范围 | 引入阶段 |
|----|------|------|---------|
| UI primitive 单测 | `vitest` + `@testing-library/svelte` | `ui/` 每个组件核心交互 | P1 |
| Resource 单测 | `vitest` | 订阅共享 / 生命周期 / error 保留 / enabled 切换 | P1 |
| Router 单测 | `vitest` | 路径匹配 / params / 子路由 / 未匹配回退 | P1 |
| Feature 组件测 | `vitest` + `@testing-library/svelte` | 复杂交互（多选 / 过滤 / 步进） | feature 迁移时 |
| E2E 黑盒 | Playwright | 连接 / 断开 / 添加 / Settings / Onboarding | P11 |
| a11y 检查 | `@axe-core/playwright` | 主要 page 无严重违规 | P11 |
| Visual regression | Playwright screenshot | light/dark 关键页快照 | P11（可选） |

**原则**：不追覆盖率，追"每个会出错的地方都有一个测试保底"。预计 60-80 个测试点。

**CI**（`gui/web` 范围）：`npm ci` → `svelte-check` → `vitest run` → `npm run build` → `playwright test`。加入现有 `.github/workflows/build.yml`。

---

## 10 · 风险与缓解

| # | 风险 | 可能性 | 影响 | 缓解 |
|---|------|--------|------|------|
| R1 | bits-ui 与 Svelte 5 runes 不兼容 | 中 | 中 | P1 端到端冒烟；不兼容时降级 melt-ui 或自写 |
| R2 | bundle size 膨胀 | 低 | 低 | P1 基线 + 每阶段 `vite build` 对比；预算 +30kb gzip |
| R3 | 长分支与 main 冲突 | 低 | 中 | 阶段化合入，rebase 而非合并 |
| R4 | Wails WebView 不支持新 CSS feature | 中 | 中 | 不用 `:has()` / subgrid；P1 做 Wails smoke |
| R5 | i18n 漏翻（硬编码英文） | 中 | 低 | CI grep 检查 `ui/` 和 `features/` 无裸英文 |
| R6 | 旧 localStorage key 残留 | 低 | 低 | P10 一次性清理（`shuttle_ui_mode` 等） |
| R7 | Bridge 期新旧样式混杂 | 中 | 低 | bridge 透明转发；过渡期 2-3 周 |
| R8 | Playwright 旧选择器失效 | 高 | 低 | P11 统一更新；过渡期跳过破损测试 |
| R9 | 新需求插队 | 中 | 中 | 属将改造 feature → 等迁移后再做；否则直接改旧代码不受影响 |

**回滚预案**：每阶段独立 revert；P3-P9 任一失败，单独 revert，shell + 其他 feature 不受影响。

---

## 11 · 参考

- Visual direction mock-ups: `.superpowers/brainstorm/*/content/visual-direction.html`, `shell-and-dashboard.html`
- Geist inspiration: vercel.com, linear.app, railway.app, shadcn
- bits-ui: https://bits-ui.com
- Svelte 5 runes: https://svelte.dev/docs/svelte/$state
