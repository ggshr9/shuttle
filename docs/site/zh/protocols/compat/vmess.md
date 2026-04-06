# VMess

## 概述

VMess 是源自 V2Ray 的协议，内置 AEAD 加密——适合连接现有 VMess 服务器；新部署建议优先使用 VLESS。

## 客户端配置

```yaml
outbounds:
  - tag: "vmess-out"
    type: "vmess"
    server: "server.example.com:443"
    uuid: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
    cipher: "aes-128-gcm"
    tls:
      enabled: true
      server_name: "server.example.com"
```

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `tag` | string | 是 | — | 该出站的唯一名称 |
| `type` | string | 是 | — | 必须为 `vmess` |
| `server` | string | 是 | — | VMess 服务器的 `host:port` |
| `uuid` | string | 是 | — | 用户 UUID |
| `cipher` | string | 否 | `aes-128-gcm` | 加密方式（见下表） |
| `alter_id` | int | 否 | `0` | 必须为 `0`（仅支持 AEAD 模式） |
| `tls.enabled` | bool | 否 | `false` | 启用 TLS |
| `tls.server_name` | string | 否 | 服务器主机名 | TLS 握手的 SNI |
| `tls.insecure` | bool | 否 | `false` | 跳过证书验证 |

**支持的加密方式：**

| 加密方式 | 备注 |
|---------|------|
| `aes-128-gcm` | 默认；在大多数平台上有硬件加速 |
| `chacha20-poly1305` | 适合没有 AES-NI 指令集的设备 |
| `none` | 不加密（仅在 TLS 上使用） |

> **注意：** Shuttle 仅支持 AEAD 模式（`alter_id: 0`），不支持遗留的基于 MD5 的 VMess。

## 服务端配置

```yaml
inbounds:
  - tag: "vmess-in"
    type: "vmess"
    listen: ":443"
    users:
      - uuid: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
        cipher: "aes-128-gcm"
    tls:
      enabled: true
      cert_file: "/path/to/cert.pem"
      key_file: "/path/to/key.pem"
```

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `tag` | string | 是 | — | 该入站的唯一名称 |
| `type` | string | 是 | — | 必须为 `vmess` |
| `listen` | string | 是 | — | 监听地址 `[addr]:port` |
| `users` | list | 是 | — | `{uuid, cipher}` 对象列表 |
| `tls.enabled` | bool | 否 | `false` | 启用 TLS |
| `tls.cert_file` | string | TLS 时必填 | — | TLS 证书路径 |
| `tls.key_file` | string | TLS 时必填 | — | TLS 私钥路径 |

## URI 格式

VMess 使用 base64 编码的 JSON 链接：

```
vmess://base64(json)
```

JSON 负载格式：

```json
{
  "v": "2",
  "ps": "名称",
  "add": "server.example.com",
  "port": "443",
  "id": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
  "aid": "0",
  "scy": "aes-128-gcm",
  "net": "tcp",
  "tls": "tls",
  "sni": "server.example.com"
}
```

## 兼容性

| 工具 | 等效配置 |
|------|---------|
| **Clash** | `type: vmess`，`cipher: aes-128-gcm`，`alterId: 0` |
| **sing-box** | `type: vmess` 出站，包含 `security` 字段 |
| **Xray** | 启用 AEAD 的 VMess 出站 |
