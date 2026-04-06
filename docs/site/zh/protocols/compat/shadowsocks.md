# Shadowsocks

## 概述

Shadowsocks 是一个轻量级 AEAD 加密代理协议——适合需要广泛生态兼容性和简单配置的场景。

## 客户端配置

```yaml
outbounds:
  - tag: "ss-out"
    type: "shadowsocks"
    server: "server.example.com:8388"
    method: "aes-256-gcm"
    password: "your-password"
```

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `tag` | string | 是 | — | 该出站的唯一名称 |
| `type` | string | 是 | — | 必须为 `shadowsocks` |
| `server` | string | 是 | — | Shadowsocks 服务器的 `host:port` |
| `method` | string | 是 | — | 加密方式（见下表） |
| `password` | string | 是 | — | 共享密钥 |

**支持的加密方式：**

| 方式 | 密钥长度 | 备注 |
|------|----------|------|
| `aes-128-gcm` | 128 位 | 在支持 AES-NI 的硬件上速度很快 |
| `aes-256-gcm` | 256 位 | 更强的安全性；在 AES-NI 硬件上性能相同 |
| `chacha20-ietf-poly1305` | 256 位 | 适合没有 AES-NI 指令集的设备 |

## 服务端配置

```yaml
inbounds:
  - tag: "ss-in"
    type: "shadowsocks"
    listen: ":8388"
    method: "aes-256-gcm"
    password: "your-password"
```

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `tag` | string | 是 | — | 该入站的唯一名称 |
| `type` | string | 是 | — | 必须为 `shadowsocks` |
| `listen` | string | 是 | — | 监听地址 `[addr]:port` |
| `method` | string | 是 | — | 必须与客户端一致 |
| `password` | string | 是 | — | 必须与客户端一致 |

## URI 格式

```
ss://base64(method:password)@host:port#名称
```

**示例：**

```
ss://YWVzLTI1Ni1nY206eW91ci1wYXNzd29yZA==@server.example.com:8388#我的服务器
```

base64 负载编码格式为 `method:password`（大多数客户端不要求填充）。

## 兼容性

| 工具 | 等效配置 |
|------|---------|
| **Clash** | `type: ss`，`cipher: aes-256-gcm` |
| **sing-box** | `type: shadowsocks`，`method: aes-256-gcm` |
| **Xray** | Shadowsocks 出站，包含 `method` 和 `password` |
