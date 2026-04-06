# fake-ip DNS

fake-ip 是专为 TUN 透明代理设计的 DNS 模式。Shuttle 不将域名解析为真实 IP，而是从保留地址段中返回一个虚假 IP，并在内部维护虚假 IP 到原始域名的映射。

## 为什么需要 fake-ip？

在 TUN 模式下，所有数据包都经过 Shuttle 虚拟网卡。如果先进行真实 DNS 解析，内核会将数据包路由到真实 IP——直连流量会绕过代理，而需要代理的流量也无法被拦截。

使用 fake-ip 的流程：

1. 应用查询 `example.com` 的 DNS。
2. Shuttle 返回 `198.18.0.1`（来自保留地址池的虚假 IP）。
3. 应用连接到 `198.18.0.1`。
4. Shuttle 拦截数据包，查找 `198.18.0.1` → `example.com` 的映射，并通过适当的出站路由。

这消除了首次连接关键路径上的一次 DNS 往返，降低感知延迟。

---

## 配置示例

```yaml
dns:
  mode: fake-ip             # 启用 fake-ip 模式（备选：normal）
  fake_ip_range: 198.18.0.0/15   # 虚假 IP 地址池
  fake_ip_filter:           # 这些域名绕过 fake-ip，返回真实 IP
    - "*.lan"
    - "*.local"
    - "*.stun.*"
    - "stun.*.*"
    - "+.stun.*.*.*"
    - "localhost"
    - "time.*.com"
    - "ntp.*.*"
    - "*.ntp.org"
  persist: false            # 重启后保留 fake-ip 映射
```

### 字段说明

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `mode` | string | `normal` | `fake-ip` 或 `normal` |
| `fake_ip_range` | CIDR | `198.18.0.0/15` | 虚假 IP 地址池 |
| `fake_ip_filter` | list | 见下方 | 返回真实 IP 的域名列表 |
| `persist` | bool | `false` | 关机时将映射保存到磁盘 |

---

## 默认过滤规则

以下模式默认加入过滤列表（返回真实 IP 响应）：

| 模式 | 原因 |
|------|------|
| `*.local`、`*.lan` | 局域网服务发现 |
| `localhost` | 回环地址 |
| `*.stun.*`、`stun.*.*` | WebRTC STUN — 需要真实 IP |
| `ntp.*.*`、`*.ntp.org` | NTP — 时钟同步需要真实 IP |
| `time.*.com` | 时间服务 |

向 `fake_ip_filter` 添加条目可扩展此列表。

---

## 已知兼容性问题

**NTP / 时间同步** — 务必过滤 NTP 服务器域名，虚假 IP 会导致 `ntpd` / `chronyd` 异常。

**STUN / WebRTC** — STUN 探测会发送源 IP，虚假 IP 会导致反射地址检测错误。默认过滤规则已覆盖常见 STUN 主机名。

**mDNS / Bonjour** — 组播 DNS 在普通 DNS 栈之外运行，fake-ip 不干扰也无影响。

**iOS / Android 网络门户检测** — 部分平台会探测特定的 Apple/Google URL，若返回虚假 IP 设备可能显示"无网络"警告。请将相关主机名加入 `fake_ip_filter`。

**内网 DNS 环境** — 如果内网 DNS 使用的私有域名未公开委派，请将这些域名加入 `fake_ip_filter`，确保它们由内网 DNS 解析而非被分配虚假 IP。
