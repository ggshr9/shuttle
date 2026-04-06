# VLESS

## 概述

VLESS 是一个没有内置加密层的轻量级代理协议——在生产环境中需搭配 TLS 或 Reality 使用以保障安全。

## 客户端配置

```yaml
outbounds:
  - tag: "vless-out"
    type: "vless"
    server: "server.example.com:443"
    uuid: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
    tls:
      enabled: true
      server_name: "server.example.com"
      # 使用 Reality 时：
      reality:
        enabled: false
        public_key: ""
        short_id: ""
```

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `tag` | string | 是 | — | 该出站的唯一名称 |
| `type` | string | 是 | — | 必须为 `vless` |
| `server` | string | 是 | — | VLESS 服务器的 `host:port` |
| `uuid` | string | 是 | — | 用于认证的用户 UUID |
| `tls.enabled` | bool | 否 | `false` | 启用 TLS |
| `tls.server_name` | string | 否 | — | TLS 握手的 SNI |
| `tls.insecure` | bool | 否 | `false` | 跳过证书验证 |
| `tls.reality.enabled` | bool | 否 | `false` | 使用 Reality 代替标准 TLS |
| `tls.reality.public_key` | string | Reality 时必填 | — | 服务端 X25519 公钥 |
| `tls.reality.short_id` | string | Reality 时必填 | — | 与服务端匹配的短 ID |

## 服务端配置

```yaml
inbounds:
  - tag: "vless-in"
    type: "vless"
    listen: ":443"
    users:
      - uuid: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
    tls:
      enabled: true
      cert_file: "/path/to/cert.pem"
      key_file: "/path/to/key.pem"
```

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `tag` | string | 是 | — | 该入站的唯一名称 |
| `type` | string | 是 | — | 必须为 `vless` |
| `listen` | string | 是 | — | 监听地址 `[addr]:port` |
| `users` | list | 是 | — | `{uuid}` 对象列表 |
| `tls.enabled` | bool | 否 | `false` | 启用 TLS |
| `tls.cert_file` | string | TLS 时必填 | — | TLS 证书路径 |
| `tls.key_file` | string | TLS 时必填 | — | TLS 私钥路径 |

## URI 格式

```
vless://uuid@host:port?security=tls&sni=server.example.com&type=tcp#名称
```

**查询参数说明：**

| 参数 | 说明 |
|------|------|
| `security` | `tls`、`reality` 或 `none` |
| `sni` | 服务器名称指示 |
| `fp` | TLS 指纹（如 `chrome`） |
| `pbk` | Reality 公钥 |
| `sid` | Reality 短 ID |
| `type` | 网络类型（`tcp`、`ws`、`grpc`） |

## 兼容性

| 工具 | 等效配置 |
|------|---------|
| **Clash** | `type: vless`，`tls: true` |
| **sing-box** | `type: vless` 出站 |
| **Xray** | VLESS 出站，配合 `streamSettings` |
