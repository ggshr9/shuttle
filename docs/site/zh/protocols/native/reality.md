# Reality（Shuttle 原生）

## 概述

Reality 是 Shuttle 的 TLS + Noise IK 传输协议，支持 SNI 域名伪装和可选的后量子加密——旨在使流量与访问真实网站的 TLS 流量无法区分。

## 客户端配置

```yaml
outbounds:
  - tag: "reality-out"
    type: "reality"
    server: "server.example.com:443"
    auth_key: "your-auth-key"
    tls:
      server_name: "www.cloudflare.com"   # 被伪装的域名
    reality:
      enabled: true
      public_key: "SERVER_X25519_PUBLIC_KEY"
      short_id: "abcdef01"
      post_quantum: false
```

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `tag` | string | 是 | — | 该出站的唯一名称 |
| `type` | string | 是 | — | 必须为 `reality` |
| `server` | string | 是 | — | Reality 服务器的 `host:port` |
| `auth_key` | string | 是 | — | Noise IK 认证密钥 |
| `tls.server_name` | string | 是 | — | 被伪装的真实网站 SNI |
| `reality.enabled` | bool | 是 | — | 必须为 `true` |
| `reality.public_key` | string | 是 | — | 服务端 X25519 公钥（base64） |
| `reality.short_id` | string | 是 | — | 与服务端匹配的短 ID（十六进制，2-16 字符） |
| `reality.post_quantum` | bool | 否 | `false` | 启用 ML-KEM 后量子密钥交换 |

## 服务端配置

```yaml
inbounds:
  - tag: "reality-in"
    type: "reality"
    listen: ":443"
    auth_key: "your-auth-key"
    reality:
      enabled: true
      private_key: "SERVER_X25519_PRIVATE_KEY"
      short_ids:
        - "abcdef01"
        - "12345678"
      dest: "www.cloudflare.com:443"     # 非 Reality 客户端的实际 TLS 转发目标
      server_names:
        - "www.cloudflare.com"
      post_quantum: false
```

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `tag` | string | 是 | — | 该入站的唯一名称 |
| `type` | string | 是 | — | 必须为 `reality` |
| `listen` | string | 是 | — | 监听地址 `[addr]:port` |
| `auth_key` | string | 是 | — | 必须与客户端一致 |
| `reality.private_key` | string | 是 | — | 服务端 X25519 私钥（base64） |
| `reality.short_ids` | list | 是 | — | 接受的客户端短 ID 列表 |
| `reality.dest` | string | 是 | — | 非 Reality TLS 连接的透传目标 |
| `reality.server_names` | list | 是 | — | 可接受的 SNI 值列表 |
| `reality.post_quantum` | bool | 否 | `false` | 启用后量子密钥交换 |

## 生成密钥对

```bash
# 为 Reality 生成 X25519 密钥对
shuttle keygen reality
# 输出：
#   private_key: <base64>
#   public_key:  <base64>
```

## 工作原理

1. 客户端使用真实热门网站的 SNI 建立 TLS 连接。
2. 如果服务端未识别出 `short_id`，服务端会将连接透明地代理到真实目标——从外部观察者的角度看，服务端的行为与 CDN 边缘节点完全一致。
3. 被识别的客户端在 TLS 会话内继续进行 Noise IK 握手，建立加密且经过认证的隧道。
4. `post_quantum: true` 增加 ML-KEM 封装步骤，为抵御量子攻击提供前向保密性。

## URI 格式

```
shuttle://reality?server=server.example.com:443&pk=PUBLIC_KEY&sid=abcdef01&sni=www.cloudflare.com#名称
```

## 兼容性

Reality 是 Shuttle 原生协议。若需 VLESS+Reality（Xray 兼容），请配置独立的 VLESS 入站。Shuttle 的 Reality 与 Xray 的 REALITY 扩展在线格式上不兼容。
