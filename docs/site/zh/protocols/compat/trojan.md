# Trojan

## 概述

Trojan 通过在 TLS 连接上使用基于密码的 SHA224 认证，将代理流量伪装成 HTTPS——适合需要让流量与正常网页流量无异的场景。

## 客户端配置

```yaml
outbounds:
  - tag: "trojan-out"
    type: "trojan"
    server: "server.example.com:443"
    password: "your-password"
    tls:
      server_name: "server.example.com"
      insecure: false
```

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `tag` | string | 是 | — | 该出站的唯一名称 |
| `type` | string | 是 | — | 必须为 `trojan` |
| `server` | string | 是 | — | Trojan 服务器的 `host:port` |
| `password` | string | 是 | — | 共享密钥（以 SHA224 哈希发送） |
| `tls.server_name` | string | 否 | 服务器主机名 | TLS 握手的 SNI |
| `tls.insecure` | bool | 否 | `false` | 跳过证书验证 |
| `tls.ca_file` | string | 否 | — | 自定义 CA 证书路径 |

## 服务端配置

```yaml
inbounds:
  - tag: "trojan-in"
    type: "trojan"
    listen: ":443"
    passwords:
      - "your-password"
    tls:
      cert_file: "/path/to/cert.pem"
      key_file: "/path/to/key.pem"
    fallback:
      host: "127.0.0.1"
      port: 80
```

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `tag` | string | 是 | — | 该入站的唯一名称 |
| `type` | string | 是 | — | 必须为 `trojan` |
| `listen` | string | 是 | — | 监听地址 `[addr]:port` |
| `passwords` | list | 是 | — | 允许的密码列表 |
| `tls.cert_file` | string | 是 | — | TLS 证书路径 |
| `tls.key_file` | string | 是 | — | TLS 私钥路径 |
| `fallback.host` | string | 否 | — | 非 Trojan 连接的转发目标主机 |
| `fallback.port` | int | 否 | — | 非 Trojan 连接的转发目标端口 |

`fallback` 选项让服务器能将无法识别的连接转发到真实 Web 服务器，使该服务从外部看起来与 HTTPS 完全相同。

## URI 格式

```
trojan://password@host:port?sni=server.example.com#名称
```

**查询参数说明：**

| 参数 | 说明 |
|------|------|
| `sni` | 服务器名称指示 |
| `allowInsecure` | `1` 表示跳过证书验证 |
| `alpn` | ALPN 协议（如 `h2,http/1.1`） |

## 兼容性

| 工具 | 等效配置 |
|------|---------|
| **Clash** | `type: trojan`，配合 `sni` |
| **sing-box** | `type: trojan` 出站 |
| **Xray** | Trojan 出站，配合 `streamSettings` |
