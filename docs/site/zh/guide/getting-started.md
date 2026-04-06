# 快速开始

## 安装

### 下载预编译二进制

在 [GitHub Releases](https://github.com/your-org/shuttle/releases) 页面下载对应平台的压缩包，解压后将可执行文件放入 `$PATH` 中的目录。

| 平台 | 文件名 |
|------|--------|
| Linux x86_64 | `shuttle-linux-amd64.tar.gz` |
| Linux arm64 | `shuttle-linux-arm64.tar.gz` |
| macOS arm64 | `shuttle-darwin-arm64.tar.gz` |
| Windows x86_64 | `shuttle-windows-amd64.zip` |

每个压缩包包含两个二进制文件：`shuttle`（客户端）和 `shuttled`（服务端）。

### 从源码编译

需要 Go 1.24 或更高版本。

```bash
git clone https://github.com/your-org/shuttle.git
cd shuttle

# 客户端
CGO_ENABLED=0 go build -o shuttle ./cmd/shuttle

# 服务端
CGO_ENABLED=0 go build -o shuttled ./cmd/shuttled
```

CLI 二进制不需要 CGo，支持通过 `GOOS`/`GOARCH` 直接交叉编译。

---

## 快速配置 — 客户端

创建 `config.yaml` 配置文件：

```yaml
# config.yaml — 最小客户端配置

# 出站服务器
outbounds:
  - name: my-server
    type: auto          # 自动协商 H3 / Reality / CDN
    server: your.server.example.com
    port: 443
    password: your-password-here

# 本地代理监听
inbounds:
  - type: socks5
    listen: 127.0.0.1
    port: 1080
  - type: http
    listen: 127.0.0.1
    port: 8080

# 路由：所有流量走代理
routing:
  default: my-server
```

启动客户端：

```bash
./shuttle -c config.yaml
```

将应用程序的代理设置为 SOCKS5 `127.0.0.1:1080`（或 HTTP 代理 `127.0.0.1:8080`）即可使用。

---

## 快速配置 — 服务端

在服务器上创建 `server.yaml`：

```yaml
# server.yaml — 最小服务端配置

listen: 0.0.0.0
port: 443
password: your-password-here

tls:
  cert: /etc/shuttle/cert.pem
  key:  /etc/shuttle/key.pem

transport:
  preferred: auto   # 同时监听 H3、Reality、CDN
```

启动服务端：

```bash
./shuttled -c server.yaml
```

测试阶段可使用自签名证书：

```bash
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 \
  -keyout key.pem -out cert.pem -days 365 -nodes \
  -subj "/CN=your.server.example.com"
```

---

## 连接第三方服务器

Shuttle 支持连接运行 Shadowsocks、VLESS、Trojan 等协议的服务器，只需在 `config.yaml` 中配置对应的出站即可：

### Shadowsocks

```yaml
outbounds:
  - name: ss-server
    type: shadowsocks
    server: ss.example.com
    port: 8388
    cipher: aes-256-gcm
    password: your-ss-password
```

### VLESS

```yaml
outbounds:
  - name: vless-server
    type: vless
    server: vless.example.com
    port: 443
    uuid: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
    tls:
      enabled: true
      server_name: vless.example.com
    transport:
      type: ws
      path: /path
```

### Trojan

```yaml
outbounds:
  - name: trojan-server
    type: trojan
    server: trojan.example.com
    port: 443
    password: your-trojan-password
    tls:
      enabled: true
      server_name: trojan.example.com
```

### Hysteria2

```yaml
outbounds:
  - name: hy2-server
    type: hysteria2
    server: hy2.example.com
    port: 443
    password: your-hy2-password
    tls:
      server_name: hy2.example.com
```

将 `routing.default` 设置为对应的出站名称即可使用。

---

## 图形界面（GUI）

**shuttle-gui** 是 Shuttle 的桌面应用，提供系统托盘图标、简单模式（快速上手）以及高级模式（包含 Mesh VPN、拥塞控制设置、实时流量图表等完整功能）。

从 [GitHub Releases](https://github.com/your-org/shuttle/releases) 页面下载 `shuttle-gui`。首次启动时会引导你添加服务器。

GUI 会自行管理配置文件并在内部启停引擎，无需单独运行 `shuttle` 进程。

---

## 下一步

- [配置参考](/zh/guide/configuration) — 所有配置项的完整说明
- [策略组](/zh/features/proxy-groups) — url-test、fallback、load-balance、quality 组
- [Mesh VPN](/zh/features/mesh-vpn) — 客户端之间的 P2P VPN
- [拥塞控制](/zh/features/congestion-control) — BBR、Brutal、Adaptive 模式
