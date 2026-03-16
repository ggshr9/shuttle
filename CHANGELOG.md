# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-03-16

### Added
- **Multi-transport proxy**: H3/QUIC, Reality/TLS+Noise, CDN/HTTP2, and WebRTC transports
- **Adaptive congestion control**: BBR, Brutal, and auto-switching based on packet loss & RTT
- **Intelligent routing**: Domain trie matching, GeoIP/GeoSite rules, DNS-over-HTTPS with caching and prefetch
- **Proxy listeners**: SOCKS5, HTTP CONNECT, TUN device with per-app routing
- **Mesh VPN**: Hub-and-spoke relay with P2P NAT traversal via STUN/hole-punching
- **Desktop GUI**: Wails + Svelte SPA with system tray support
- **Server features**: Prometheus metrics, audit logging, admin API, graceful two-phase shutdown
- **Security**: Config encryption at rest, HMAC transport auth, post-quantum KEM support, SSRF prevention
- **Deployment**: Docker one-click deploy, OpenWrt package, multi-platform release builds (Linux/macOS/Windows/FreeBSD)
- **Diagnostics**: Connection tracing, speed test, diagnostics bundle export
- **CI/CD**: GitHub Actions with lint, test, build, coverage, and automated release with SHA256 checksums
