# Phase 5: Documentation Site + Migration Guide

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create a bilingual (Chinese/English) documentation site with protocol docs, feature guides, API reference, and a Clash migration guide.

**Architecture:** VitePress static site deployed to GitHub Pages. Matches project's Vite ecosystem. Organized by: guide (getting started, config, migration), protocols (one page each), features (groups, providers, fake-ip, mesh, CC, multipath), API reference.

**Tech Stack:** VitePress 1.x, Node.js 22+, GitHub Actions for deployment

**Spec:** `docs/superpowers/specs/2026-04-05-ecosystem-compatibility-design.md` — Section 7

**Depends on:** Phase 1-4 (documents features built in earlier phases)

---

## File Structure

### New Files
```
docs/site/
├── package.json
├── .vitepress/
│   └── config.ts                      — VitePress config with i18n (en/zh)
├── en/
│   ├── index.md                       — Landing page
│   ├── guide/
│   │   ├── getting-started.md         — Install + first connection
│   │   ├── configuration.md           — Full config reference
│   │   └── migrate-from-clash.md      — Clash → Shuttle migration guide
│   ├── protocols/
│   │   ├── shadowsocks.md
│   │   ├── vless.md
│   │   ├── trojan.md
│   │   ├── hysteria2.md
│   │   ├── vmess.md
│   │   ├── wireguard.md
│   │   ├── h3.md
│   │   ├── reality.md
│   │   └── cdn.md
│   ├── features/
│   │   ├── proxy-groups.md
│   │   ├── providers.md
│   │   ├── fake-ip.md
│   │   ├── mesh-vpn.md
│   │   ├── congestion-control.md
│   │   └── multipath.md
│   └── api/
│       └── rest-api.md
├── zh/                                — Chinese mirror (same structure)
│   ├── index.md
│   ├── guide/
│   ├── protocols/
│   ├── features/
│   └── api/
└── .github/
    └── workflows/
        └── docs.yml                   — GitHub Actions: build + deploy to Pages
```

---

### Task 1: VitePress Project Setup

**Files:**
- Create: `docs/site/package.json`
- Create: `docs/site/.vitepress/config.ts`

- [ ] **Step 1: Initialize VitePress project**

```bash
cd docs/site
npm init -y
npm install -D vitepress
```

- [ ] **Step 2: Create package.json scripts**

```json
{
  "name": "shuttle-docs",
  "private": true,
  "scripts": {
    "dev": "vitepress dev",
    "build": "vitepress build",
    "preview": "vitepress preview"
  },
  "devDependencies": {
    "vitepress": "^1.5.0"
  }
}
```

- [ ] **Step 3: Create VitePress config with i18n**

```ts
// docs/site/.vitepress/config.ts
import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'Shuttle',
  description: 'Multi-transport network toolkit',

  locales: {
    en: {
      label: 'English',
      lang: 'en',
      themeConfig: {
        nav: [
          { text: 'Guide', link: '/en/guide/getting-started' },
          { text: 'Protocols', link: '/en/protocols/shadowsocks' },
          { text: 'Features', link: '/en/features/proxy-groups' },
          { text: 'API', link: '/en/api/rest-api' },
        ],
        sidebar: {
          '/en/guide/': [
            { text: 'Getting Started', link: '/en/guide/getting-started' },
            { text: 'Configuration', link: '/en/guide/configuration' },
            { text: 'Migrate from Clash', link: '/en/guide/migrate-from-clash' },
          ],
          '/en/protocols/': [
            { text: 'Shuttle Native', items: [
              { text: 'H3 (HTTP/3)', link: '/en/protocols/h3' },
              { text: 'Reality', link: '/en/protocols/reality' },
              { text: 'CDN', link: '/en/protocols/cdn' },
            ]},
            { text: 'Compatible', items: [
              { text: 'Shadowsocks', link: '/en/protocols/shadowsocks' },
              { text: 'VLESS', link: '/en/protocols/vless' },
              { text: 'Trojan', link: '/en/protocols/trojan' },
              { text: 'Hysteria2', link: '/en/protocols/hysteria2' },
              { text: 'VMess', link: '/en/protocols/vmess' },
              { text: 'WireGuard', link: '/en/protocols/wireguard' },
            ]},
          ],
          '/en/features/': [
            { text: 'Proxy Groups', link: '/en/features/proxy-groups' },
            { text: 'Providers', link: '/en/features/providers' },
            { text: 'fake-ip DNS', link: '/en/features/fake-ip' },
            { text: 'Mesh VPN', link: '/en/features/mesh-vpn' },
            { text: 'Congestion Control', link: '/en/features/congestion-control' },
            { text: 'Multipath', link: '/en/features/multipath' },
          ],
          '/en/api/': [
            { text: 'REST API', link: '/en/api/rest-api' },
          ],
        },
      },
    },
    zh: {
      label: '中文',
      lang: 'zh-CN',
      themeConfig: {
        nav: [
          { text: '指南', link: '/zh/guide/getting-started' },
          { text: '协议', link: '/zh/protocols/shadowsocks' },
          { text: '功能', link: '/zh/features/proxy-groups' },
          { text: 'API', link: '/zh/api/rest-api' },
        ],
        sidebar: {
          '/zh/guide/': [
            { text: '快速开始', link: '/zh/guide/getting-started' },
            { text: '配置参考', link: '/zh/guide/configuration' },
            { text: '从 Clash 迁移', link: '/zh/guide/migrate-from-clash' },
          ],
          '/zh/protocols/': [
            { text: 'Shuttle 原生', items: [
              { text: 'H3 (HTTP/3)', link: '/zh/protocols/h3' },
              { text: 'Reality', link: '/zh/protocols/reality' },
              { text: 'CDN', link: '/zh/protocols/cdn' },
            ]},
            { text: '兼容协议', items: [
              { text: 'Shadowsocks', link: '/zh/protocols/shadowsocks' },
              { text: 'VLESS', link: '/zh/protocols/vless' },
              { text: 'Trojan', link: '/zh/protocols/trojan' },
              { text: 'Hysteria2', link: '/zh/protocols/hysteria2' },
              { text: 'VMess', link: '/zh/protocols/vmess' },
              { text: 'WireGuard', link: '/zh/protocols/wireguard' },
            ]},
          ],
          '/zh/features/': [
            { text: '策略组', link: '/zh/features/proxy-groups' },
            { text: 'Provider', link: '/zh/features/providers' },
            { text: 'fake-ip DNS', link: '/zh/features/fake-ip' },
            { text: 'Mesh VPN', link: '/zh/features/mesh-vpn' },
            { text: '拥塞控制', link: '/zh/features/congestion-control' },
            { text: '多路径', link: '/zh/features/multipath' },
          ],
          '/zh/api/': [
            { text: 'REST API', link: '/zh/api/rest-api' },
          ],
        },
      },
    },
  },
})
```

- [ ] **Step 4: Verify dev server starts**

```bash
cd docs/site && npm run dev
```
Expected: VitePress dev server starts on localhost.

- [ ] **Step 5: Commit**

```bash
git add docs/site/package.json docs/site/.vitepress/
git commit -m "feat(docs): initialize VitePress documentation site with i18n"
```

---

### Task 2: Landing Page + Getting Started

**Files:**
- Create: `docs/site/en/index.md`
- Create: `docs/site/en/guide/getting-started.md`
- Create: `docs/site/zh/index.md`
- Create: `docs/site/zh/guide/getting-started.md`

- [ ] **Step 1: Write English landing page**

```markdown
---
layout: home

hero:
  name: Shuttle
  text: Multi-Transport Network Toolkit
  tagline: Adaptive congestion control, mesh VPN, multipath — with full protocol compatibility
  actions:
    - theme: brand
      text: Get Started
      link: /en/guide/getting-started
    - theme: alt
      text: Protocols
      link: /en/protocols/shadowsocks

features:
  - title: Multi-Protocol Support
    details: Shadowsocks, VLESS, Trojan, Hysteria2, VMess, WireGuard — plus Shuttle's own H3, Reality, and CDN transports
  - title: Adaptive Congestion Control
    details: BBR, Brutal, and Adaptive modes. Auto-switches based on packet loss and RTT for optimal performance under interference
  - title: Mesh VPN
    details: P2P NAT traversal with STUN, hole punching, UPnP, and TURN fallback. Connect your devices directly
  - title: Multipath
    details: Aggregate bandwidth across multiple transports simultaneously with weighted, min-latency, or load-balance scheduling
  - title: Strategy Groups
    details: url-test, fallback, select, load-balance, and quality (congestion-aware) groups with Proxy Providers
  - title: Cross-Platform
    details: Windows, macOS, Linux, Android, iOS. Native GUI with system tray, real-time bandwidth curves, and mesh management
---
```

- [ ] **Step 2: Write Getting Started guide**

Cover: install (binary download / go install / GUI), first client config, connect, verify.

```markdown
# Getting Started

## Install

### Binary Download
Download from [GitHub Releases](https://github.com/xxx/shuttle/releases):
- `shuttle` — CLI client
- `shuttled` — CLI server
- `shuttle-gui` — Desktop GUI (Windows/macOS/Linux)

### Build from Source
\`\`\`bash
# Client
CGO_ENABLED=0 go build -o shuttle ./cmd/shuttle

# Server
CGO_ENABLED=0 go build -o shuttled ./cmd/shuttled
\`\`\`

## Quick Start (Client)

1. Create config file `config.yaml`:

\`\`\`yaml
server:
  addr: "your-server.com:443"
  password: "your-password"

transport:
  preferred: "auto"
  reality:
    enabled: true
    server_name: "your-server.com"

proxy:
  socks5:
    enabled: true
    addr: ":1080"
  http:
    enabled: true
    addr: ":8080"

routing:
  default: "proxy"
\`\`\`

2. Run: `shuttle -c config.yaml`
3. Set your browser proxy to `127.0.0.1:1080` (SOCKS5) or `127.0.0.1:8080` (HTTP)

## Quick Start (Server)

1. Create server config:

\`\`\`yaml
listen: ":443"
auth:
  password: "your-password"
tls:
  cert_file: "/path/to/cert.pem"
  key_file: "/path/to/key.pem"
transport:
  reality:
    enabled: true
\`\`\`

2. Run: `shuttled -c server.yaml`
```

- [ ] **Step 3: Write Chinese versions**

Mirror the English content in `zh/` directory.

- [ ] **Step 4: Commit**

```bash
git add docs/site/en/index.md docs/site/en/guide/ docs/site/zh/
git commit -m "docs: add landing page and getting started guide (en/zh)"
```

---

### Task 3: Protocol Documentation (all 9 protocols)

**Files:**
- Create: `docs/site/en/protocols/*.md` (9 files)
- Create: `docs/site/zh/protocols/*.md` (9 files)

Each protocol page follows this template:

```markdown
# [Protocol Name]

## Overview
One-sentence description and typical use case.

## Client Configuration

\`\`\`yaml
outbounds:
  - tag: "example"
    type: "[protocol]"
    server: "server.example.com:port"
    # ... protocol-specific fields with explanations
\`\`\`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| ... | ... | ... | ... |

## Server Configuration

\`\`\`yaml
inbounds:
  - tag: "example-in"
    type: "[protocol]"
    listen: ":port"
    # ... server-specific fields
\`\`\`

## URI Format

\`\`\`
protocol://...
\`\`\`

Used for subscription sharing links.

## Compatibility

| Tool | Config equivalent |
|------|------------------|
| Clash | `type: [x]` in `proxies:` |
| sing-box | `type: "[x]"` in `outbounds` |
| Xray | `protocol: "[x]"` in `outbounds` |
```

- [ ] **Step 1: Write all 9 protocol docs**

Create each file with the full config reference for that protocol. Include all fields from the config types defined in Phase 2 and Phase 4.

Protocol list: shadowsocks, vless, trojan, hysteria2, vmess, wireguard, h3, reality, cdn.

- [ ] **Step 2: Write Chinese versions**

- [ ] **Step 3: Commit**

```bash
git add docs/site/en/protocols/ docs/site/zh/protocols/
git commit -m "docs: add protocol documentation for all 9 supported protocols (en/zh)"
```

---

### Task 4: Feature Documentation

**Files:**
- Create: `docs/site/en/features/*.md` (6 files)
- Create: `docs/site/zh/features/*.md` (6 files)

- [ ] **Step 1: Write proxy-groups.md**

Cover: url-test, fallback, select, load-balance, quality strategies. Config examples. GUI usage. Nesting. Health check config.

- [ ] **Step 2: Write providers.md**

Cover: Proxy Provider (config, auto-format detection, filter). Rule Provider (domain/ipcidr/classical behaviors). Hot-reload. Caching.

- [ ] **Step 3: Write fake-ip.md**

Cover: What fake-ip is and why. Config. Filter list. Known compatibility issues.

- [ ] **Step 4: Write mesh-vpn.md**

Cover: Architecture (hub-spoke + P2P). Config. NAT traversal techniques. Virtual IP assignment.

- [ ] **Step 5: Write congestion-control.md**

Cover: BBR vs Brutal vs Adaptive. When to use each. Config. How adaptive switching works.

- [ ] **Step 6: Write multipath.md**

Cover: Aggregate, weighted, min-latency scheduling. Config. Use cases.

- [ ] **Step 7: Commit**

```bash
git add docs/site/en/features/ docs/site/zh/features/
git commit -m "docs: add feature documentation (groups, providers, fake-ip, mesh, CC, multipath)"
```

---

### Task 5: Clash Migration Guide

**Files:**
- Create: `docs/site/en/guide/migrate-from-clash.md`
- Create: `docs/site/zh/guide/migrate-from-clash.md`

- [ ] **Step 1: Write migration guide**

```markdown
# Migrate from Clash

## Concept Mapping

| Clash | Shuttle | Notes |
|-------|---------|-------|
| `proxies:` | `outbounds:` | Same concept, different key |
| `proxy-groups:` | `outbounds:` with `type: "group"` | Groups and proxies share the same list |
| `rules:` | `routing.rule_chain:` | More powerful AND/OR logic |
| `proxy-providers:` | `proxy_providers:` | Same concept |
| `rule-providers:` | `rule_providers:` | Same concept, supports domain/ipcidr/classical |
| `dns.fake-ip-range` | `routing.dns.fake_ip_range` | Same concept |
| `tun.enable` | `proxy.tun.enabled` | Same concept |
| `url-test` group | `strategy: "url-test"` | With tolerance_ms |
| `fallback` group | `strategy: "failover"` | Same behavior |
| `select` group | `strategy: "select"` | With API + GUI |
| `load-balance` group | `strategy: "loadbalance"` | Round-robin |

## Config Conversion Example

### Clash
\`\`\`yaml
proxies:
  - name: "hk-01"
    type: ss
    server: hk.example.com
    port: 8388
    cipher: aes-256-gcm
    password: "test"

proxy-groups:
  - name: "auto"
    type: url-test
    proxies: ["hk-01"]
    url: "http://www.gstatic.com/generate_204"
    interval: 300

rules:
  - DOMAIN-SUFFIX,google.com,auto
  - GEOIP,CN,DIRECT
  - MATCH,auto
\`\`\`

### Shuttle
\`\`\`yaml
outbounds:
  - tag: "hk-01"
    type: "shadowsocks"
    server: "hk.example.com:8388"
    method: "aes-256-gcm"
    password: "test"

  - tag: "auto"
    type: "group"
    strategy: "url-test"
    outbounds: ["hk-01"]
    health_check:
      url: "http://www.gstatic.com/generate_204"
      interval: "300s"
      tolerance_ms: 50

routing:
  rule_chain:
    - match: { domain_suffix: ["google.com"] }
      action: "auto"
    - match: { geoip: ["CN"] }
      action: "direct"
  default: "auto"
\`\`\`

## Importing Subscriptions

Your existing Clash subscription URLs work directly in Shuttle:

\`\`\`yaml
proxy_providers:
  - name: "my-sub"
    url: "https://your-clash-subscription-url"
    interval: "3600s"
\`\`\`

Shuttle auto-detects Clash YAML format and parses all proxy types.

## What's Different

### Shuttle has but Clash doesn't
- **Quality strategy group** — congestion-aware node selection (packet loss + RTT)
- **Adaptive congestion control** — auto-switches between BBR and Brutal
- **Mesh VPN** — P2P NAT traversal with STUN + hole punching
- **Multipath** — aggregate bandwidth across multiple transports
- **Post-quantum encryption** — Reality transport with X25519+ML-KEM-768
- **Strategy group nesting** — groups can reference other groups

### Clash has but Shuttle doesn't (yet)
- **Script/Starlark rules** — not planned
- **TProxy mode** — use TUN instead
- **Relay chain** — use WireGuard outbound for chaining
```

- [ ] **Step 2: Write Chinese version**

- [ ] **Step 3: Commit**

```bash
git add docs/site/en/guide/migrate-from-clash.md docs/site/zh/guide/migrate-from-clash.md
git commit -m "docs: add Clash migration guide with concept mapping and config examples"
```

---

### Task 6: API Reference

**Files:**
- Create: `docs/site/en/api/rest-api.md`
- Create: `docs/site/zh/api/rest-api.md`

- [ ] **Step 1: Write API reference**

Document all endpoints from `gui/api/routes_*.go` including the new group and provider endpoints from Phase 1.

Format: endpoint, method, description, request body, response body, example curl command.

- [ ] **Step 2: Commit**

```bash
git add docs/site/en/api/ docs/site/zh/api/
git commit -m "docs: add REST API reference documentation"
```

---

### Task 7: Configuration Reference

**Files:**
- Create: `docs/site/en/guide/configuration.md`
- Create: `docs/site/zh/guide/configuration.md`

- [ ] **Step 1: Write full config reference**

Document every field in `ClientConfig` and `ServerConfig` with types, defaults, and descriptions. Organized by section (server, transport, proxy, routing, outbounds, providers, mesh, etc.).

- [ ] **Step 2: Commit**

```bash
git add docs/site/en/guide/configuration.md docs/site/zh/guide/configuration.md
git commit -m "docs: add full configuration reference"
```

---

### Task 8: GitHub Actions Deployment

**Files:**
- Create: `docs/site/.github/workflows/docs.yml`

- [ ] **Step 1: Create GitHub Actions workflow**

```yaml
# docs/site/.github/workflows/docs.yml
name: Deploy Documentation

on:
  push:
    branches: [main]
    paths: ['docs/site/**']
  workflow_dispatch:

permissions:
  contents: read
  pages: write
  id-token: write

concurrency:
  group: pages
  cancel-in-progress: false

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: 22
          cache: npm
          cache-dependency-path: docs/site/package-lock.json
      - run: cd docs/site && npm ci && npm run build
      - uses: actions/upload-pages-artifact@v3
        with:
          path: docs/site/.vitepress/dist

  deploy:
    environment:
      name: github-pages
      url: ${{ steps.deployment.outputs.page_url }}
    needs: build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/deploy-pages@v4
        id: deployment
```

- [ ] **Step 2: Verify build succeeds**

```bash
cd docs/site && npm run build
```
Expected: Build completes, output in `.vitepress/dist/`.

- [ ] **Step 3: Commit**

```bash
git add docs/site/.github/
git commit -m "ci: add GitHub Actions workflow for documentation deployment"
```
