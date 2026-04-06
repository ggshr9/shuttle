# Hysteria2

## 概述

Hysteria2 是基于 QUIC 的协议，使用 Brutal 拥塞控制——最适合高丢包或带宽受限的网络环境，在这些场景下基于 TCP 的协议表现不佳。

## 客户端配置

```yaml
outbounds:
  - tag: "hy2-out"
    type: "hysteria2"
    server: "server.example.com:443"
    password: "your-password"
    tls:
      server_name: "server.example.com"
      insecure: false
    bandwidth:
      up: "50 mbps"
      down: "200 mbps"
```

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `tag` | string | 是 | — | 该出站的唯一名称 |
| `type` | string | 是 | — | 必须为 `hysteria2` |
| `server` | string | 是 | — | Hysteria2 服务器的 `host:port` |
| `password` | string | 是 | — | 认证密码 |
| `tls.server_name` | string | 否 | 服务器主机名 | TLS 握手的 SNI |
| `tls.insecure` | bool | 否 | `false` | 跳过证书验证 |
| `tls.ca_file` | string | 否 | — | 自定义 CA 证书路径 |
| `bandwidth.up` | string | 否 | — | 上行带宽提示（如 `50 mbps`） |
| `bandwidth.down` | string | 否 | — | 下行带宽提示（如 `200 mbps`） |

提供准确的带宽提示有助于 Brutal 拥塞控制达到最优吞吐量。单位支持：`bps`、`kbps`、`mbps`、`gbps`。

## 服务端配置

```yaml
inbounds:
  - tag: "hy2-in"
    type: "hysteria2"
    listen: ":443"
    passwords:
      - "your-password"
    tls:
      cert_file: "/path/to/cert.pem"
      key_file: "/path/to/key.pem"
    bandwidth:
      up: "1 gbps"
      down: "1 gbps"
```

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `tag` | string | 是 | — | 该入站的唯一名称 |
| `type` | string | 是 | — | 必须为 `hysteria2` |
| `listen` | string | 是 | — | 监听地址 `[addr]:port` |
| `passwords` | list | 是 | — | 允许的密码列表 |
| `tls.cert_file` | string | 是 | — | TLS 证书路径 |
| `tls.key_file` | string | 是 | — | TLS 私钥路径 |
| `bandwidth.up` | string | 否 | — | 服务器上行容量 |
| `bandwidth.down` | string | 否 | — | 服务器下行容量 |

## URI 格式

```
hysteria2://password@host:port?sni=server.example.com#名称
```

**查询参数说明：**

| 参数 | 说明 |
|------|------|
| `sni` | 服务器名称指示 |
| `insecure` | `1` 表示跳过证书验证 |
| `up` | 上行带宽提示 |
| `down` | 下行带宽提示 |

## 兼容性

| 工具 | 等效配置 |
|------|---------|
| **Clash** | `type: hysteria2`（Meta/Premium 版本） |
| **sing-box** | `type: hysteria2` 出站 |
| **Xray** | 原生不支持 |
