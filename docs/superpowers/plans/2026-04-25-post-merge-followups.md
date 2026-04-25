# 后续计划 — iOS VPN-mode SPA 工作的收尾

记录 2026-04-24/25 这两天产出的工作之后还需要完成的事。按优先级 + 依赖关系组织。

## 目前进行中（待 review / merge）

### PR #9 — `feat(ios): VPN-mode SPA replacement via DataAdapter + envelope IPC`
- 分支：`feat/ios-vpn-spa`
- 规模：42 commits，56 files，+3287 / −65
- Phase 1–6.3 全部落地。完整的 spec + plan + 实现 + tests + 文档
- 仅有的未完成 task：6.4 (Phase γ cleanup)，明确 gated 在 TestFlight 72h
- CI 当前状态：所有可执行的检查都绿；mobile build 受 PR #10 阻塞
- 下一步：human review → merge

### PR #10 — `fix(ci): unblock gomobile bind + arm32 splice portability + lint`
- 分支：`fix/ci-gomobile-bind`
- 规模：5 commits，~50 行
- 三个修复：tools.go for gomobile bind / `unix.Splice` arm32 / `-checklinkname=0` for `wlynxg/anet`
- CI 当前状态：bind + cross-compile + link 都通过；mobile builds 现在卡在 platform scaffolding（gradle wrapper / xcodeproj 缺失）
- 下一步：human review → merge → rebase #9 on top

---

## 立即跟进（PR #10 merge 之后）

### A. Mobile platform scaffolding — Android
- 任务：`cd mobile/android && gradle wrapper --gradle-version 8.5`
- 提交：`gradlew`, `gradlew.bat`, `gradle/wrapper/`
- 这一项做完之后 Android APK CI 就能完整产出 `.apk`
- 工作量：~15 分钟一次性脚手架

### B. Mobile platform scaffolding — iOS
- 任务：在 Xcode 里建 `mobile/ios/Shuttle.xcodeproj`
- 需要的 targets：`Shuttle`、`ShuttleExtension`、`ShuttleTests`（新增 in PR #9）、`ShuttleUITests`
- 配置：
  - link `SharedBridge` SPM 到 `Shuttle` 和 `ShuttleExtension`
  - Network Extension 的 entitlements + App Group `group.com.shuttle.app`
  - `Shuttle/www/` 加进 Bundle resources
  - `Shuttle.entitlements` / `ShuttleExtension.entitlements`（已存在）
  - `ShuttleTests` 的 host-target = `Shuttle`，启用 `@testable import`
- 工作量：1–2 小时手工 Xcode 操作（不是代码工作）
- 提交后：CI 才能跑 `xcodebuild test`，APIBridgeTests / testSPALoadsInVPNMode 才有意义

---

## TestFlight β 阶段（PR #9 + PR #10 都 merge 之后）

### Phase β 工作清单

参考 `docs/mobile-smoke.md` 新增的 `## iOS VPN mode` 部分。门槛 criteria 在同文件的 `### Phase γ acceptance gate`。

操作步骤：
1. 在 #B 完成的 Xcode 项目里 archive → upload to TestFlight
2. 至少 3 个 dev 设备 × 2 个 tester 跑 12 项 smoke checklist
3. 监控 7 天：Apple Crashes dashboard / GitHub issues / 直接反馈
4. 每个测试设备每 24h 检查一次 Settings → Diagnostics 计数器（<1 fallback trigger）
5. 1 个设备做 24h 内存 spot-check（<40 MB extension memory）

预计时间：5–7 天 wall clock + ~2 工作日修 bug。

### Settings → Diagnostics counter（Phase γ 前置）

`docs/mobile-smoke.md` 的 Phase γ gate 引用了"on-device counter"，但当前代码没有这个面板。在 Phase β 开始之前需要补：
- `lib/data/connection-state.ts` 已经追踪 `ok` / `error`；导出累计的 fallback trigger 计数
- Settings 页加一个新 sub-page `Diagnostics`，显示：bridge 失败次数、上次失败时间、平均 RTT
- Per spec §11.3 — 仅本地，无远程上报

工作量：~半天 TS。

---

## Phase γ 清理（Task 6.4 — gated）

`docs/superpowers/plans/2026-04-24-ios-vpn-mode-spa.md#task-64-phase-γ-cleanup-—-remove-fallback-html-and-legacy-string-commands` 已经写好详细清单。简要：

触发条件：上面 TestFlight β 的所有 5 条 criteria 同时满足。

要做的事：
1. 删除 `mobile/ios/Shuttle/FallbackHandler.swift`
2. `mobile/ios/Shuttle/ShuttleApp.swift`：移除 `fallbackHandler` 字段、`createVPNControlHTML()` 方法、bundle-missing fallback 分支
3. `mobile/ios/ShuttleExtension/PacketTunnelProvider.swift`：删除 `handleAppMessage` 的 legacy string-command 分支
4. `gui/web/src/app/boot.ts`：删除 `?bridge=0` debug 分支（保留 `?bridge=1`）

工作量：1 天。这是单独的一个 PR。

---

## 技术债务清扫（按 review 中标记 "deferred to a later sweep"）

可以单独一个 PR 一起做，估计 1 工作日：

### 来自 Phase 3 review（Go API 层）
- `EventQueue.Wait` 改用 `context.AfterFunc`（Go 1.21+）替代当前手写 goroutine + Broadcast 模式 — 减少每次 cond wakeup 的 allocation
- `EventQueue.Push` 应该 log `json.Marshal` 失败而不是静默吞掉
- `/ws/events` 当前没有专门的 WebSocket handler 测试；需要 `httptest.NewServer` + `coder/websocket.Dial` 的端到端测试
- `pumpEngineEvents` 接受 ctx 参数支持 graceful shutdown（避免测试中 goroutine 泄漏）
- `EventQueue.Tail` 的 `out` slice 容量按 `min(events_since, max)` 而不是当前的 `events_since`

### 来自 Phase 4 review（已在 PR 内修了 critical 项；这些是 deferred 项）
- 简化 conformance 测试中冗长的 per-adapter branching — 抽 `drivesValue<T>` helper
- BridgeTransport 类身只是 send 转发；考虑改成 module-level `getBridge()` 函数
- 加 BridgeAdapter "auth header 已存在不覆盖" test（HttpAdapter 同样缺）

### 来自 Phase 5 review（也已修 critical 项）
- 迁移 URLSession.shared 到 URLSessionDataDelegate，在 streaming 阶段就检查 192KB 上限（防御 extension OOM）
- `apiBridge: APIBridge!` / `fallbackHandler: FallbackHandler!` 改成 explicit Optional + guard
- `shuttleBridgeBootstrapJS` 改成 `static let` on `APIBridge`（scoped）
- `forwardToLocalAPI` 当 `Data(base64Encoded:)` 失败时应该返回 transport error 而不是发送空 body

### 来自 Phase 6 review（CI 修完之后做）
- `ShuttleTests/README.md` — 5-step "怎么把这个 target 加进 xcodeproj"
- 加 `testCompleteJS_EscapesU2028U2029` test（需要 WKWebView mock，moderate 工作量）
- Phase 6.1 的 `testHandle_NilResponseInvokesSendClosure` 名字过度承诺；要么改名要么强化断言

---

## CI 持续问题（不是这次工作引入的，但应该处理）

### sandbox-test 一直 fail
- 自 2026-04-08 以来 main 分支上一直 fail
- 5 个 shell 集成测试失败：`test_client_a_to_server` / `test_socks5_proxy` / `test_http_proxy` / `test_socks5_get_endpoint` / `test_client_b_socks5`
- 都是代理连通性测试，可能是 docker 环境配置漂移或上游 dep 问题
- 单独一个 issue / PR 调查

### `wlynxg/anet` 长期方案
- 目前用 `-checklinkname=0` 临时绕过
- 真正的修复路径：
  1. 等 anet 上游修复（v0.0.6+，目前没有）
  2. fork 并提交 PR 到 anet
  3. 找替代的库（直接用 stdlib `net` 在 Android 上的 fallback 路径）
- 不紧急；checklinkname=0 在 Go 1.x 整个生命周期都会 work

---

## 优先级排序（建议）

```
P0：PR #10 review + merge        ← 最快解锁 mobile CI
P0：PR #9 review + merge         ← 把 SPA 工作落地

P1：mobile platform scaffolding   ← 解锁 TestFlight 路径
    ├── Android gradle wrapper（15 分钟）
    └── iOS xcodeproj（1–2 小时手工）

P1：Settings → Diagnostics 面板   ← Phase β 前置（spec §11.3）

P2：TestFlight β + 7 天 soak     ← 等待数据，不是代码

P2：Phase γ cleanup (Task 6.4)   ← 数据满足之后

P3：Tech debt sweep                ← 集中一个 PR
P3：sandbox-test 调查              ← 单独一个 PR
P3：anet 长期方案                  ← issue / 等上游
```

---

## 时间预估总览

| 阶段 | 工作量 | wall time |
|---|---|---|
| review + merge PR #9 + #10 | 半天 | 取决于 reviewer |
| mobile scaffolding (Android + iOS) | 1 工作日 | 1 天 |
| Diagnostics 面板 | 半天 | 半天 |
| TestFlight β β 等待 | <1 工作日（修 bug） | 5–7 天 |
| Phase γ cleanup | 1 工作日 | 1 天 |
| Tech debt sweep | 1 工作日 | 1 天 |
| 合计 | ~5 工作日 | ~10 天 wall |

---

## 当前会话产出清单

- 2026-04-24 spec：`docs/superpowers/specs/2026-04-24-ios-vpn-mode-spa-design.md`
- 2026-04-24 plan：`docs/superpowers/plans/2026-04-24-ios-vpn-mode-spa.md`
- 2026-04-25 followups（本文件）

代码：
- 整个 `gui/web/src/lib/data/` 模块（DataAdapter + 两个实现 + conformance 套件）
- `gui/api/events.go`、`gui/api/routes_events.go`、`gui/api/healthz.go`、engine pump
- `mobile/ios/SharedBridge/` SPM
- `mobile/ios/Shuttle/APIBridge.swift`、`FallbackHandler.swift`
- `mobile/ios/ShuttleTests/APIBridgeTests.swift`
- ShuttleApp + PacketTunnelProvider + VPNManager 的 envelope 改造
- `tools.go`（mobile CI 修复）
- `internal/relay/splice_linux.go`（arm32 修复）
- `build/scripts/build-android.sh` / `build-ios.sh`（checklinkname 修复）

236+ TS tests / 89+ Go tests 全绿。
