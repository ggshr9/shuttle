# Security

> **Last reviewed:** 2026-04-28. This document is reviewed at every release.

## Verifying Download Integrity

Every release includes a `checksums.txt` file containing SHA-256 hashes for all release artifacts. To verify a downloaded file:

**Linux / macOS:**

```bash
# Download the checksums file and the binary you need
curl -LO https://github.com/shuttleX/shuttle/releases/download/<tag>/checksums.txt
curl -LO https://github.com/shuttleX/shuttle/releases/download/<tag>/shuttle-linux-amd64

# Verify the hash
sha256sum -c checksums.txt --ignore-missing
```

**Windows (PowerShell):**

```powershell
# After downloading checksums.txt and the binary
(Get-FileHash .\shuttle-windows-amd64.exe -Algorithm SHA256).Hash
# Compare the output with the corresponding entry in checksums.txt
```

If the hash does not match, do not use the file. Re-download from the official release page and verify again.

## Reporting Security Issues

**Confidential reports:** Use [GitHub Security Advisory](https://github.com/shuttleX/shuttle/security/advisories/new) for any security-sensitive issue. This is the preferred channel — reports are private until coordinated disclosure.

**Non-sensitive concerns:** A public [GitHub issue](https://github.com/shuttleX/shuttle/issues/new) is fine for hardening suggestions, dependency updates, or configuration questions where no exploit path is involved.

**PGP:** No project PGP key is currently published. Please use GitHub Security Advisory for confidential reports — GitHub encrypts reports in transit and at rest.

**What to include:**
- Affected version (commit hash if running from main)
- Steps to reproduce
- Estimated impact (data exposure / denial of service / privilege escalation)
- Suggested fix if you have one

We aim to acknowledge reports within 72 hours and to ship a fix or mitigation within 30 days for high-severity issues.

## Supported Versions

Security fixes are applied to the latest release only. We recommend always running the most recent version of Shuttle to benefit from the latest security patches and improvements.

## Threat Model

Shuttle is designed to defend against the following classes of adversary:

**In scope:**
- Passive traffic analysis on the wire between client and server.
- Active SNI probing of the server's TLS endpoint (Reality transport).
- Passive deep packet inspection identifying or fingerprinting Shuttle traffic.
- Unauthorised access to the management plane (`/api/*` endpoints).
- Unauthorised use of forwarded outbound traffic (e.g., open-relay abuse).

**Out of scope:**
- Local-host compromise (device theft, root-level malware on client or server).
- Active collaboration by the upstream CDN, hosting provider, or transit network.
- Long-term confidentiality breach by quantum computation against current Noise IK key exchange.
- Side-channel attacks against the TLS implementation provided by the Go standard library.

**Trust boundaries:**

```
[client app] ⟷ [shuttle CLI / GUI] ⟷ [transport: H3/Reality/CDN] ⟷ [shuttled] ⟷ [destination]
                       │                                                   │
                       └─── management plane (/api/*) ── separate trust ───┘
                                                              domain (admin.token)
```

The management plane is its own trust domain: its credentials must not be derivable from or reused with the data-plane credentials (`auth.password`, `auth.private_key`).

## Hardening Checklist

Treat this as a pre-deploy checklist. Items marked **(default)** are configured automatically by the install scripts; others require explicit configuration.

**Process & filesystem**
- [ ] Service runs as a dedicated non-root user (default — `shuttle` user via `install.sh`)
- [ ] systemd `ProtectSystem=strict`, `NoNewPrivileges=true`, `PrivateTmp=true` (default)
- [ ] `CapabilityBoundingSet=CAP_NET_BIND_SERVICE` only (default)
- [ ] Config file mode `0600`, directory `0700`, owned by service user
- [ ] TLS private key file mode `0600`

**Authentication**
- [ ] `auth.password` is at least 16 chars, randomly generated (use `openssl rand -base64 32`)
- [ ] For Reality, prefer `auth.private_key` over passwords entirely
- [ ] `admin.token` is at least 32 chars, randomly generated, never reused as `auth.password`

**Network exposure**
- [ ] Admin port (`admin.listen`) bound to `127.0.0.1` or restricted by firewall to operator networks
- [ ] Metrics port (`metrics.listen`) bound to `127.0.0.1` plus token; never exposed publicly
- [ ] Public listener ports limited to the transports actually in use

**Routing & SSRF**
- [ ] `router.allow_private_networks: false` in production (the sandbox-only override defaults to `false` already)
- [ ] If `cdn` outbound is enabled, quotas are configured to bound proxy abuse risk

**Observability**
- [ ] IP reputation rate-limiting enabled (default — auto-bans after 5 failed auth attempts)
- [ ] Logs do not echo `Authorization` header, password, or private key (verified by spot-check at each release)
- [ ] Audit log destination configured if compliance requires it
