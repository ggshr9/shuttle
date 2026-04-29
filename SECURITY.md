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
