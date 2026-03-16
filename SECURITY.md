# Security

## Verifying Download Integrity

Every release includes a `checksums.txt` file containing SHA-256 hashes for all release artifacts. To verify a downloaded file:

**Linux / macOS:**

```bash
# Download the checksums file and the binary you need
curl -LO https://github.com/shuttle-proxy/shuttle/releases/download/<tag>/checksums.txt
curl -LO https://github.com/shuttle-proxy/shuttle/releases/download/<tag>/shuttle-linux-amd64

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

If you discover a security vulnerability, please report it by opening a GitHub issue:

https://github.com/shuttle-proxy/shuttle/issues/new

Include as much detail as possible: affected version, steps to reproduce, and potential impact. We will respond promptly and coordinate a fix.

## Supported Versions

Security fixes are applied to the latest release only. We recommend always running the most recent version of Shuttle to benefit from the latest security patches and improvements.
