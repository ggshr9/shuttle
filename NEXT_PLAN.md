# Shuttle 下一步完善计划

## 已完成功能回顾

### 第一阶段 ✅
- 流量统计展示（Dashboard 流量卡片）
- 一键导入服务器（支持 base64/JSON/shuttle:// URI）
- 配置导出（JSON/YAML/URI 格式）

### 第二阶段 ✅
- 节点测速（TCP+TLS 延迟测量）
- WebSocket 实时测速流
- 按延迟排序

### 第三阶段 ✅
- 订阅管理（添加/刷新/删除）
- SIP008 格式支持
- 自动刷新

### 第四阶段 ✅
- 多语言支持（中/英文）
- 无刷新切换语言

### 第五阶段 ✅
- 日志导出
- 历史流量统计（持久化存储）

### 第六阶段 ✅
- 服务器自动选择（根据延迟自动选最佳节点）
- 连接状态通知（浏览器 Notification API）
- 键盘快捷键（Cmd/Ctrl+K 快速切换连接）

### 第七阶段 ✅
- 规则导入/导出（JSON 格式）
- 规则模板（绕过中国、全局代理、直连全部、广告拦截）
- DNS 设置界面（国内/国外 DNS、DoH、缓存设置）

### 第八阶段 ✅
- Android VPN Service（前台服务通知、JavaScript 桥接、状态监控）
- iOS Network Extension（PacketTunnelProvider、VPNManager、App Group 共享）

### 第九阶段 ✅
- Windows TUN 支持（WinTun 集成）
- Windows 路由表配置（netsh/route 命令）
- 网格路由支持

### 第十阶段 ✅
- 流量统计图表（Canvas 实时曲线图，无外部依赖）
- 连接日志详情（可展开的日志条目，显示目标、规则、协议、流量）
- 配置备份/恢复（完整备份含服务器、订阅、规则、设置）
- 自动更新（检查 GitHub Releases，显示更新日志和下载链接）

---

## 功能开发完成

所有计划的功能阶段已完成实现。

---

## 优先级说明

| 级别 | 含义 | 建议时间 |
|------|------|----------|
| P1 | 重要功能，建议实现 | 2-3周 |
| P2 | 锦上添花，可选实现 | 按需 |

## 工作量评估

| 任务 | 工作量 | 说明 |
|------|--------|------|
| 小 | 1-2天 | 单文件改动，逻辑简单 |
| 中 | 3-5天 | 多文件改动，需要设计 |
| 大 | 1-2周 | 新功能模块，需要调研 |

---

## 第十一阶段 ✅ Mesh VPN P2P 优化

### 已完成
- **NAT-PMP 支持** - 除 UPnP 外支持 NAT-PMP 协议 (Apple 路由器等)
- **并行协议发现** - UPnP 和 NAT-PMP 并行尝试，加速发现
- **平台特定网关检测** - macOS/Linux/Windows 读取路由表
- **路径缓存** - 记忆成功的连接方法，加速重连
- **连接质量监控** - RTT/抖动/丢包率追踪
- **零配置体验** - P2P 启用时自动进行 NAT 穿透优化
- **端口伪装** - 支持 DNS(53)/HTTPS(443)/IKE(500) 端口伪装

---

## 第十二阶段 ✅ P2P 协议扩展

### 已完成
- **PCP 协议支持** - 支持 Port Control Protocol (RFC 6887, NAT-PMP 继任者)
- **三协议并行发现** - UPnP、NAT-PMP、PCP 并行尝试，使用最先成功的协议
- **P2P 集成测试** - 全面的端到端测试覆盖
- **mDNS 局域网发现** - 支持 mDNS 协议发现局域网内的其他 Shuttle 客户端
- **TURN Relay 支持** - 实现 TURN 客户端 (RFC 5766/8656) 作为 P2P 最终回退

---

## 第十三阶段 ✅ ICE Restart 支持

### 已完成
- **ICE 状态机** - 实现完整的 ICE Gathering/Connection 状态机 (RFC 8445)
- **ICE Restart 信令** - 添加 SignalICERestart 消息类型支持凭证交换
- **ICE Restart 触发** - 支持手动、网络变化、质量降低、超时等触发原因
- **连接质量监控** - 基于 RTT/丢包率自动触发 ICE Restart
- **ICE Restart 冷却期** - 防止频繁重启的冷却机制
- **完整测试覆盖** - ICE 状态机和 Restart 功能的单元测试

---

## 第十四阶段 ✅ Trickle ICE 支持

### 已完成
- **Trickle ICE 采集器** - 支持增量 ICE 候选发现 (RFC 8838)
- **异步候选回调** - 发现候选时立即通知，无需等待采集完成
- **Trickle 信令协议** - SignalTrickleCandidate 和 SignalEndOfCandidates 消息类型
- **STUN 绑定请求/响应** - 构建和解析 STUN 消息获取服务器反射候选
- **ICEAgent 集成** - ICEAgent 支持 trickle 模式，可增量添加远程候选
- **P2P Manager 集成** - 信令处理和候选转发
- **完整测试覆盖** - TrickleICEGatherer 和集成测试

---

## 第十五阶段 ✅ 多 STUN 负载均衡 & IPv6 支持

### 已完成
- **多 STUN 并行查询** - 同时查询多个 STUN 服务器，返回最快响应
- **QueryParallel/QueryAllParallel** - 新增并行查询 API
- **QueryParallelWithConn** - 共享连接的并行查询 (NAT 检测用)
- **IPv6 IP 分配器** - 支持 ULA 地址分配 (fd00::/8)
- **双栈分配器** - DualStackAllocator 同时管理 IPv4/IPv6 地址
- **IPv6 STUN 查询** - QueryIPv6/QueryParallelIPv6/QueryDualStack
- **IPv6 候选采集** - ICE 候选支持 IPv6 地址家族
- **双栈 ICE 采集** - GatherDualStack 同时采集 IPv4/IPv6 候选
- **完整测试覆盖** - STUN 并行查询和 IPv6 测试

---

## 第十六阶段 ✅ WebRTC DataChannel 传输

### 已完成
- **WebRTC Client/Server** - 完整的 WebRTC DataChannel 传输实现
- **HTTP/WebSocket 信令** - 支持 HTTP POST 和 WebSocket 两种信令方式
- **Trickle ICE** - 增量 ICE 候选发现和交换
- **自动重连** - 连接失败时自动重连机制
- **连接统计** - RTT、丢包率、带宽统计收集
- **yamux 多路复用** - DataChannel 上的流多路复用
- **完整测试覆盖** - 端到端、多流、大数据传输测试

---

## 第十七阶段 ✅ 技术债务清理

### 已完成
- **A11y 修复** - 所有 modal/overlay 添加 ARIA 属性 (role, aria-modal, aria-labelledby)
- **键盘支持** - 所有对话框支持 Escape 键关闭
- **TypeScript 迁移** - 前端 JS 文件迁移到 TypeScript
- **Toast 通知** - 统一的 toast 通知系统替代内联 msg 显示
- **错误处理** - 改进错误提示和用户反馈

---

## 第十八阶段 ✅ 连接质量图表

### 已完成
- **连接质量图表** - Dashboard 显示 P2P 连接 RTT/丢包/抖动历史
- **Mesh 状态展示** - 显示虚拟 IP、网络 CIDR、对等节点列表
- **实时质量监控** - RTT、Jitter、PacketLoss、Score 实时更新
- **多对等节点支持** - 支持同时监控多个 mesh 节点的连接质量

---

## 第十九阶段 ✅ QoS 流量优先级标记

### 已完成
- **QoS 分类器** - 基于端口、协议、域名、进程的流量分类
- **优先级级别** - Critical/High/Normal/Bulk/Low 五级优先级
- **DSCP 标记** - TUN 模式下为 IP 包标记 DSCP 值 (EF/AF41/AF21/AF11/BE)
- **默认端口规则** - SSH、RDP、SIP 等端口的默认优先级
- **GUI 设置** - QoS 开关和自定义规则配置界面
- **完整测试覆盖** - QoS 分类器单元测试

---

## 下一步计划

### P2 - 可选优化

| 功能 | 描述 | 工作量 |
|------|------|--------|
| ~~**多路径传输**~~ | ✅ 已实现 | - |
| ~~**带宽聚合**~~ | ✅ 已实现 | - |
| ~~**连接质量图表**~~ | ✅ 已实现 | - |
| ~~**QoS 标记**~~ | ✅ 已实现 | - |
| **Mesh 拓扑可视化** | GUI 显示 mesh 网络拓扑图 | 中 |

---

## 技术债务清理

1. ~~**A11y 警告修复**~~ ✅ - 修复 modal overlay 的无障碍访问警告
2. **测试覆盖** - 为新增模块添加单元测试
3. ~~**错误处理**~~ ✅ - 统一错误处理和用户提示 (Toast 通知系统)
4. ~~**TypeScript 迁移**~~ ✅ - 前端 lib 模块已迁移到 TypeScript
5. ~~**P2P 集成测试**~~ ✅ - 已添加端到端 P2P 连接测试

---

## 当前架构

### Mesh VPN 架构

```
┌──────────────────────────────────────────────────────────────────┐
│                        Shuttle Mesh VPN                          │
├──────────────────────────────────────────────────────────────────┤
│                                                                   │
│  Client A                                            Client B    │
│  ┌──────────┐                                      ┌──────────┐  │
│  │ P2P Mgr  │                                      │ P2P Mgr  │  │
│  │ ──────── │                                      │ ──────── │  │
│  │ UPnP     │ ◄───────── mDNS Discovery ─────────► │ UPnP     │  │
│  │ NAT-PMP  │ ◄───────── Direct P2P ─────────────► │ NAT-PMP  │  │
│  │ PCP      │                                      │ PCP      │  │
│  │ STUN     │ ◄───────── TURN Relay ─────────────► │ STUN     │  │
│  │ mDNS     │                                      │ mDNS     │  │
│  │ TURN     │                                      │ TURN     │  │
│  └────┬─────┘                                      └────┬─────┘  │
│       │                  ┌──────┐                       │        │
│       └────── relay ───► │ Hub  │ ◄───── relay ────────┘        │
│                          └──────┘                                │
│                                                                   │
├──────────────────────────────────────────────────────────────────┤
│                        连接优先级                                 │
├──────────────────────────────────────────────────────────────────┤
│  1. mDNS (局域网直连)  →  零延迟本地发现                          │
│  2. UPnP/NAT-PMP/PCP   →  端口映射，并行尝试                      │
│  3. STUN + Hole Punch  →  NAT 穿透直连                            │
│  4. TURN Relay         →  RFC 5766 标准中继                       │
│  5. Server Hub Relay   →  通过 Shuttle 服务器中继                 │
└──────────────────────────────────────────────────────────────────┘
```

### 架构改进建议

| 功能 | 状态 | 说明 |
|------|------|------|
| TURN 服务器 | ✅ 已实现 | RFC 5766/8656 TURN 客户端 |
| mDNS 发现 | ✅ 已实现 | 局域网内自动发现其他客户端 |
| PCP 协议 | ✅ 已实现 | NAT-PMP 继任者，RFC 6887 |
| ICE Restart | ✅ 已实现 | 网络变化时自动重新协商连接 |
| Trickle ICE | ✅ 已实现 | RFC 8838 增量候选发现 |
| 多 STUN 负载均衡 | ✅ 已实现 | 并行查询多个 STUN 服务器 |
| IPv6 支持 | ✅ 已实现 | 双栈支持，IPv6 候选采集 |
| WebRTC 数据通道 | ✅ 已实现 | 可选的 WebRTC DataChannel 传输 |
