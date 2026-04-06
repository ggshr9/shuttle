# WireGuard

## 概述

WireGuard 是一个现代 VPN 协议，攻击面极小，性能接近内核速度——作为客户端出站将流量通过 WireGuard VPN 端点进行隧道传输。

> **仅客户端。** Shuttle 将 WireGuard 作为出站传输使用，没有 WireGuard 入站。服务端请使用官方 `wg-quick` 或 WireGuard 服务端实现。

## 客户端配置

```yaml
outbounds:
  - tag: "wg-out"
    type: "wireguard"
    private_key: "CLIENT_PRIVATE_KEY_BASE64"
    addresses:
      - "10.0.0.2/32"
      - "fd00::2/128"
    dns:
      - "1.1.1.1"
      - "8.8.8.8"
    mtu: 1420
    peers:
      - public_key: "SERVER_PUBLIC_KEY_BASE64"
        endpoint: "server.example.com:51820"
        allowed_ips:
          - "0.0.0.0/0"
          - "::/0"
        keepalive: 25
```

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `tag` | string | 是 | — | 该出站的唯一名称 |
| `type` | string | 是 | — | 必须为 `wireguard` |
| `private_key` | string | 是 | — | 客户端私钥（base64） |
| `addresses` | list | 是 | — | 带前缀长度的客户端隧道 IP 地址 |
| `dns` | list | 否 | 系统默认 | 隧道内使用的 DNS 服务器 |
| `mtu` | int | 否 | `1420` | 隧道 MTU |
| `peers` | list | 是 | — | 对端配置列表（见下表） |

**对端字段：**

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `public_key` | string | 是 | — | 对端公钥（base64） |
| `pre_shared_key` | string | 否 | — | 可选的预共享密钥，提供额外安全性 |
| `endpoint` | string | 是 | — | 对端的 `host:port` |
| `allowed_ips` | list | 是 | — | 通过该对端路由的 CIDR 列表 |
| `keepalive` | int | 否 | `0` | 持久保活间隔（秒） |

## 生成密钥对

```bash
# 生成密钥对
wg genkey | tee privatekey | wg pubkey > publickey

cat privatekey   # 填入 private_key
cat publickey    # 提供给服务器管理员
```

## URI 格式

WireGuard 没有标准的订阅 URI 格式。请使用标准 `wg-quick` 的 `.conf` 文件格式导入配置，或手动填写各字段。

## 兼容性

| 工具 | 等效配置 |
|------|---------|
| **Clash** | `type: wireguard`（仅 Meta 版本） |
| **sing-box** | `type: wireguard` 出站 |
| **Xray** | 不支持 |
