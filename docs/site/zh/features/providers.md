# Provider

Provider 允许 Shuttle 从远端 URL 或本地文件拉取节点列表和规则集，让配置文件保持精简，同时接入外部订阅。

---

## 代理 Provider（Proxy Provider）

代理 Provider 拉取服务器列表。Shuttle 自动识别格式：Clash YAML、sing-box JSON、Base64 编码的 SIP002/URI 列表，或纯文本 URI 列表。

### 配置示例

```yaml
proxy_providers:
  - name: my-sub
    url: https://sub.example.com/clash.yaml
    interval: 3600          # 每小时刷新一次
    filter: "HK|SG"         # 只保留名称匹配此正则的节点
    health_check:
      url: https://www.gstatic.com/generate_204
      interval: 300

  - name: local-list
    path: ./servers.yaml    # 本地文件，无需 interval
    filter: ""
```

### 字段说明

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `name` | string | 必填 | Provider 名称，在策略组中引用 |
| `url` | string | — | 远端订阅 URL |
| `path` | string | — | 本地文件路径（二选一，不可同时填写） |
| `interval` | int | `3600` | 刷新间隔（秒） |
| `filter` | string | `""` | 对节点名称应用的正则过滤，空字符串保留全部 |
| `health_check.url` | string | generate_204 | 延迟探测 URL |
| `health_check.interval` | int | `300` | 探测间隔（秒） |

### 格式自动识别

Shuttle 按以下顺序尝试各格式：

1. Clash YAML（包含 `proxies:` 键）
2. sing-box JSON（包含 `outbounds` 数组）
3. Base64 — 解码后重新解析
4. 纯 URI 列表 — 每行一条 `ss://` / `vmess://` / `vless://` / `trojan://`

### 本地缓存

已下载的内容缓存到磁盘，远端不可达时代理仍可正常启动。缓存文件与配置文件同目录，命名为 `.<name>.cache`。

### 在策略组中引用

通过 `use` 字段在策略组中引用 Provider：

```yaml
proxy_groups:
  - tag: auto
    type: url-test
    use:
      - my-sub        # 拉取 Provider 中所有（经过滤的）节点
    interval: 300
```

---

## 规则 Provider（Rule Provider）

规则 Provider 拉取匹配规则列表（域名、IP 段或经典规则），注入到路由流程中。

### 配置示例

```yaml
rule_providers:
  - name: gfw-domains
    url: https://rules.example.com/gfw.yaml
    behavior: domain          # domain | ipcidr | classical
    interval: 86400
    format: yaml              # yaml | text

  - name: cn-cidr
    url: https://rules.example.com/cn-ipcidr.yaml
    behavior: ipcidr
    interval: 86400
```

### 行为类型

| 行为 | 匹配内容 | 条目示例 |
|------|---------|---------|
| `domain` | 精确域名或后缀 | `google.com` |
| `ipcidr` | IPv4/IPv6 CIDR | `8.8.8.0/24` |
| `classical` | 完整 Clash 风格规则 | `DOMAIN-SUFFIX,google.com` |

### 热重载

规则 Provider 在后台刷新，Shuttle 以原子方式替换规则集，不影响正在进行的连接。

### 在路由中引用

在 `match` 块中使用 `rule_provider`：

```yaml
routing:
  rules:
    - match:
        rule_provider: ["gfw-domains", "extra-domains"]
      outbound: my-proxy

    - match:
        rule_provider: ["cn-cidr"]
      outbound: DIRECT

    - outbound: my-proxy    # 默认规则
```

列表中多个 Provider 名称之间为 OR 关系。
