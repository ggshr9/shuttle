# Build & Packaging

This directory contains build configurations and packaging scripts for all supported platforms.

## Directory Structure

```
build/
├── package/
│   ├── nfpm.yaml              # Linux package config (deb/rpm/apk)
│   ├── linux/
│   │   ├── shuttle.service    # systemd service (client)
│   │   ├── shuttled.service   # systemd service (server)
│   │   ├── postinstall.sh     # Package post-install script
│   │   └── preremove.sh       # Package pre-remove script
│   ├── openwrt/
│   │   ├── Makefile           # OpenWrt SDK package definition
│   │   └── files/
│   │       ├── shuttle.init   # procd init script (client)
│   │       └── shuttled.init  # procd init script (server)
│   ├── darwin/                # macOS packaging (future)
│   └── windows/               # Windows installer (future)
└── scripts/
    ├── build-all.sh           # Build all platforms locally
    └── compress-upx.sh        # UPX compression for embedded
```

## Quick Build

### All Platforms (requires Go 1.24+)

```bash
./build/scripts/build-all.sh v1.0.0
```

### Single Platform

```bash
# Linux amd64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o shuttle ./cmd/shuttle

# OpenWrt MIPS (soft-float)
CGO_ENABLED=0 GOOS=linux GOARCH=mipsle GOMIPS=softfloat go build -ldflags="-s -w" -o shuttle ./cmd/shuttle

# macOS ARM64
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o shuttle ./cmd/shuttle

# Windows
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o shuttle.exe ./cmd/shuttle
```

## Linux Packages (deb/rpm)

Requires [nFPM](https://nfpm.goreleaser.com/):

```bash
# Install nFPM
go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest

# Build binaries first
./build/scripts/build-all.sh v1.0.0

# Create packages
VERSION=v1.0.0 GOARCH=amd64 nfpm pkg --packager deb --target dist/
VERSION=v1.0.0 GOARCH=amd64 nfpm pkg --packager rpm --target dist/
```

## OpenWrt Packages

### Using OpenWrt SDK

```bash
# 1. Download SDK for your target
wget https://downloads.openwrt.org/releases/23.05.0/targets/ramips/mt7621/openwrt-sdk-*.tar.xz

# 2. Extract and enter SDK
tar xf openwrt-sdk-*.tar.xz && cd openwrt-sdk-*

# 3. Clone shuttle to package directory
git clone https://github.com/ggshr9/shuttle package/shuttle

# 4. Update feeds and build
./scripts/feeds update -a
./scripts/feeds install -a
make package/shuttle/compile V=s
```

### Pre-built Binaries

For quick deployment without SDK:

```bash
# Build on host machine
CGO_ENABLED=0 GOOS=linux GOARCH=mipsle GOMIPS=softfloat \
  go build -ldflags="-s -w" -o shuttle ./cmd/shuttle

# Optional: compress with UPX
upx --best shuttle

# Copy to router
scp shuttle root@192.168.1.1:/usr/bin/
scp config/client.example.yaml root@192.168.1.1:/etc/shuttle/client.yaml

# Create init script on router
cat > /etc/init.d/shuttle << 'EOF'
#!/bin/sh /etc/rc.common
START=99
STOP=10
USE_PROCD=1
start_service() {
    procd_open_instance
    procd_set_param command /usr/bin/shuttle run -c /etc/shuttle/client.yaml
    procd_set_param respawn
    procd_close_instance
}
EOF
chmod +x /etc/init.d/shuttle
/etc/init.d/shuttle enable
/etc/init.d/shuttle start
```

## Binary Size Optimization

| Platform | Uncompressed | UPX Compressed |
|----------|-------------|----------------|
| linux/amd64 | ~12 MB | ~4 MB |
| linux/mipsle | ~9 MB | ~3 MB |
| linux/arm | ~10 MB | ~3.5 MB |

```bash
# Compress embedded binaries
./build/scripts/compress-upx.sh
```

## CI/CD

GitHub Actions automatically builds all platforms on:
- Push to `main` branch
- Pull requests
- Tag pushes (`v*`)

Release artifacts are uploaded to GitHub Releases on tag push.

## Supported Platforms

| Platform | Architecture | CLI | Server | GUI |
|----------|-------------|-----|--------|-----|
| Linux | amd64, arm64, arm, mips, mipsle | ✅ | ✅ | ✅ |
| macOS | amd64, arm64 | ✅ | ✅ | ✅ |
| Windows | amd64 | ✅ | ✅ | ✅ |
| FreeBSD | amd64 | ✅ | ✅ | ❌ |
| Android | arm64, arm | ✅ (AAR) | ❌ | ✅ (App) |
| iOS | arm64 | ✅ (xcframework) | ❌ | ✅ (App) |
| OpenWrt | mips, mipsle, arm | ✅ | ✅ | ❌ |
