# CDN（Shuttle 原生）

## 概述

CDN 是 Shuttle 的 HTTP/2 或 gRPC 传输协议，专为穿透 CDN 网络（Cloudflare、Fastly 等）而设计——适合直连被封锁、但 CDN 代理的 HTTPS 流量可以正常通行的场景。

## 客户端配置

```yaml
outbounds:
  - tag: "cdn-out"
    type: "cdn"
    server: "server.example.com:443"
    auth_key: "your-auth-key"
    cdn:
      mode: "h2"                          # h2 或 grpc
      domain: "server.example.com"
      path: "/stream"
      front_domain: "allowed.cdn.com"     # 可选的域前置主机
    tls:
      server_name: "allowed.cdn.com"
      insecure_skip_verify: false
```

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `tag` | string | 是 | — | 该出站的唯一名称 |
| `type` | string | 是 | — | 必须为 `cdn` |
| `server` | string | 是 | — | IP 地址或 CDN 边缘地址，含端口 |
| `auth_key` | string | 是 | — | HMAC 认证密钥 |
| `cdn.mode` | string | 否 | `h2` | 传输子模式：`h2` 或 `grpc` |
| `cdn.domain` | string | 是 | — | 发送给 CDN 的 `Host` 头 / gRPC authority |
| `cdn.path` | string | 否 | `/` | HTTP 路径（h2）或 gRPC 服务路径 |
| `cdn.front_domain` | string | 否 | — | SNI 与 Host 分离的域前置覆盖 |
| `tls.server_name` | string | 否 | `cdn.domain` | TLS 握手的 SNI（使用域前置时设为 `front_domain`） |
| `tls.insecure_skip_verify` | bool | 否 | `false` | 跳过证书验证 |

**模式说明：**

| 模式 | 说明 |
|------|------|
| `h2` | 基于 TLS 的 HTTP/2 流式传输；与标准 CDN 行为兼容 |
| `grpc` | gRPC 流式传输；部分 CDN 允许 gRPC 透传（用于 WebRTC/API 流量） |

## 服务端配置

```yaml
inbounds:
  - tag: "cdn-in"
    type: "cdn"
    listen: ":443"
    auth_key: "your-auth-key"
    cdn:
      mode: "h2"
      path: "/stream"
    tls:
      cert_file: "/path/to/cert.pem"
      key_file: "/path/to/key.pem"
```

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `tag` | string | 是 | — | 该入站的唯一名称 |
| `type` | string | 是 | — | 必须为 `cdn` |
| `listen` | string | 是 | — | 监听地址 `[addr]:port` |
| `auth_key` | string | 是 | — | 必须与客户端一致 |
| `cdn.mode` | string | 否 | `h2` | 必须与客户端一致 |
| `cdn.path` | string | 否 | `/` | 必须与客户端一致 |
| `tls.cert_file` | string | 是 | — | TLS 证书路径 |
| `tls.key_file` | string | 是 | — | TLS 私钥路径 |

## 域前置配置

域前置允许流量看上去是发往某个热门 CDN 域名，实际上却到达你自己的服务器：

1. 将服务器放在 CDN 后面（例如 Cloudflare），配置代理的 A 记录。
2. 将 `cdn.front_domain` 设为同一 CDN 上的热门域名（例如 `www.cloudflare.com`）。
3. 将 `tls.server_name` 设为 `front_domain`——SNI 发给 CDN。
4. 将 `cdn.domain` 设为你的真实域名——`Host` 头在 CDN 内部路由。

```yaml
cdn:
  mode: "h2"
  domain: "myserver.example.com"
  front_domain: "www.cloudflare.com"
tls:
  server_name: "www.cloudflare.com"
```

> **注意：** 域前置策略因 CDN 而异，请查阅所用 CDN 服务商的服务条款。

## URI 格式

```
shuttle://cdn?server=IP:443&key=KEY&domain=myserver.example.com&mode=h2#名称
```

## 兼容性

CDN 是 Shuttle 原生协议。其他工具中存在类似 WebSocket + TLS 的传输方式，但线格式不兼容。
