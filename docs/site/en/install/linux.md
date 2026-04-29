# Linux Install

This guide covers installing `shuttled` (the server daemon) on a Linux VPS.

For the desktop GUI, see the [Releases page](https://github.com/shuttleX/shuttle/releases).

## Prerequisites
- Linux (Debian, Ubuntu, RHEL, Alpine, OpenWrt, ...)
- Root access (`sudo`)
- A public IP or domain pointing to your server

## Quick install

```bash
curl -fsSL https://raw.githubusercontent.com/shuttleX/shuttle/main/scripts/install-linux.sh | sudo bash
```

The wizard will:
1. Detect your CPU architecture and download the matching binary.
2. Detect your public IP, or let you specify a domain.
3. Generate a strong random password (or accept yours).
4. Pick transports (H3, Reality, both).
5. Register a hardened systemd service and start it.

## Non-interactive install

```bash
SHUTTLE_DOMAIN=proxy.example.com \
SHUTTLE_PASSWORD=$(openssl rand -base64 32) \
SHUTTLE_TRANSPORT=both \
sudo bash -c "$(curl -fsSL https://raw.githubusercontent.com/shuttleX/shuttle/main/scripts/install-linux.sh) install --auto"
```

## Manage

```bash
systemctl status  shuttled     # state
systemctl restart shuttled     # restart after config edit
journalctl -u     shuttled -f  # live logs
```

## Upgrade and uninstall

```bash
sudo bash install.sh upgrade v0.4.1
sudo bash install.sh uninstall
```

## Read next
- [SECURITY.md](https://github.com/shuttleX/shuttle/blob/main/SECURITY.md) — pre-deploy checklist.
