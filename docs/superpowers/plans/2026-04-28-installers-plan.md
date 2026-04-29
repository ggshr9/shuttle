# Plan 3 — Cross-Platform CLI Installers

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provide single-command CLI install paths for macOS (Homebrew tap) and Windows (PowerShell), reaching the same coverage as the existing Linux `deploy/install.sh`. Reorganise the README install section so users can pick the right installer in one glance, and avoid the "wrong installer" confusion that comes from having both GUI and CLI distributions.

**Architecture:**
- A new public repository `shuttleX/homebrew-shuttle` hosts two formulae (`shuttle.rb`, `shuttled.rb`). The release workflow auto-PRs version bumps via `mislav/bump-homebrew-formula-action`.
- A new `scripts/install-windows.ps1` mirrors `deploy/install.sh` step-for-step (architecture detection, downloads, three-step wizard, service registration, firewall rules with explicit prompt, status/uninstall/upgrade subcommands).
- The existing `deploy/install.sh` is kept in place. A thin wrapper `scripts/install-linux.sh` is added so the three platforms have a consistent `scripts/install-<os>.<ext>` URL pattern.
- README install section gains a decision table at the top: "If you want X → use Y."

**Tech Stack:** Bash 5, PowerShell 7-compatible, Ruby (for Homebrew formulae), GitHub Actions.

**Spec reference:** `docs/superpowers/specs/2026-04-28-production-readiness-yellow-lights-design.md` (Workstream 5).

---

## File Structure

**Created (in this repo):**
- `scripts/install-windows.ps1` — Windows CLI installer.
- `scripts/install-linux.sh` — thin wrapper that `exec`s `deploy/install.sh`.
- `scripts/install-smoke.md` — manual smoke checklist for each platform.
- `scripts/test-install-windows.ps1` — PSScriptAnalyzer + dry-run lint.

**Created (in a separate repo `shuttleX/homebrew-shuttle`):**
- `Formula/shuttle.rb` — Homebrew formula for the `shuttle` CLI.
- `Formula/shuttled.rb` — Homebrew formula for the `shuttled` daemon.
- `README.md` — tap usage instructions.
- `.github/workflows/test.yml` — `brew test-bot` audit on PRs.

**Modified:**
- `.github/workflows/release.yml` — appends a step that PRs the homebrew tap.
- `README.md` — install section reorganised with decision table + three platform tabs.
- `docs/site/en/install/{linux,macos,windows}.md` — per-platform deploy guides.
- `docs/site/zh/install/{linux,macos,windows}.md` — Chinese versions.

---

## Task 1: Linux installer wrapper

**Files:**
- Create: `scripts/install-linux.sh`

- [ ] **Step 1.1: Implement**

```bash
#!/usr/bin/env bash
# scripts/install-linux.sh
# Thin wrapper for deploy/install.sh, providing URL parity
# (scripts/install-{linux,macos,windows}.{sh,ps1}).
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec "${SCRIPT_DIR}/../deploy/install.sh" "$@"
```

```bash
chmod +x scripts/install-linux.sh
```

- [ ] **Step 1.2: Verify the wrapper works**

```bash
./scripts/install-linux.sh status   # exits non-zero if shuttled isn't installed, that's expected
./scripts/install-linux.sh --help   # should print deploy/install.sh's usage
```

Expected: usage prints, no shell errors.

- [ ] **Step 1.3: Commit**

```bash
git add scripts/install-linux.sh
git commit -m "feat(scripts): add install-linux.sh wrapper for URL parity"
```

---

## Task 2: Windows installer skeleton

**Files:**
- Create: `scripts/install-windows.ps1`

- [ ] **Step 2.1: Write the skeleton with arg parsing**

```powershell
#Requires -Version 5.1
<#
.SYNOPSIS
  Shuttle Server installer for Windows.
.DESCRIPTION
  Mirrors deploy/install.sh: download → wizard → register service → firewall.
  Subcommands: install (default), uninstall, upgrade [version], status.
.EXAMPLE
  iwr -useb https://raw.githubusercontent.com/shuttleX/shuttle/main/scripts/install-windows.ps1 | iex
.EXAMPLE
  .\install-windows.ps1 install
.EXAMPLE
  .\install-windows.ps1 upgrade v0.4.1
#>
[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet('install', 'uninstall', 'upgrade', 'status')]
    [string]$Action = 'install',

    [Parameter(Position = 1)]
    [string]$Version = 'latest',

    [switch]$Auto,
    [string]$Domain = $env:SHUTTLE_DOMAIN,
    [string]$Password = $env:SHUTTLE_PASSWORD,
    [ValidateSet('h3', 'reality', 'both')]
    [string]$Transport = $(if ($env:SHUTTLE_TRANSPORT) { $env:SHUTTLE_TRANSPORT } else { 'both' })
)

$ErrorActionPreference = 'Stop'

$REPO         = 'shuttleX/shuttle'
$INSTALL_DIR  = "$env:ProgramFiles\Shuttle"
$CONFIG_DIR   = "$env:ProgramData\Shuttle"
$SERVICE_NAME = 'shuttled'

function Write-Info($msg)  { Write-Host "▸ $msg" -ForegroundColor Green }
function Write-Warn($msg)  { Write-Host "▸ $msg" -ForegroundColor Yellow }
function Write-Err($msg)   { Write-Host "✗ $msg" -ForegroundColor Red; exit 1 }

function Assert-Admin {
    $isAdmin = ([Security.Principal.WindowsPrincipal]`
        [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(`
        [Security.Principal.WindowsBuiltInRole]::Administrator)
    if (-not $isAdmin) { Write-Err 'Run this script in an elevated PowerShell window (Run as Administrator).' }
}

# --- main dispatch (filled in by later tasks) ---
switch ($Action) {
    'install'   { Assert-Admin; Install-Shuttled }
    'uninstall' { Assert-Admin; Uninstall-Shuttled }
    'upgrade'   { Assert-Admin; Upgrade-Shuttled $Version }
    'status'    { Show-Status }
}
```

- [ ] **Step 2.2: Lint**

```powershell
Invoke-ScriptAnalyzer -Path scripts/install-windows.ps1 -Severity Warning
```

If `Invoke-ScriptAnalyzer` is not installed:
```powershell
Install-Module -Name PSScriptAnalyzer -Scope CurrentUser -Force
```

Expected: no errors. Warnings about undefined `Install-Shuttled`/`Uninstall-Shuttled`/etc. are expected and resolved in later tasks.

- [ ] **Step 2.3: Commit**

```bash
git add scripts/install-windows.ps1
git commit -m "feat(scripts): install-windows.ps1 skeleton with arg parsing"
```

---

## Task 3: Windows — platform detection and download

**Files:**
- Modify: `scripts/install-windows.ps1`

- [ ] **Step 3.1: Add detection and download functions**

Insert before the dispatch switch:

```powershell
function Get-Architecture {
    switch ($env:PROCESSOR_ARCHITECTURE) {
        'AMD64' { 'amd64' }
        'ARM64' { 'arm64' }
        default { Write-Err "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
    }
}

function Get-PublicIP {
    foreach ($svc in 'https://api.ipify.org', 'https://ifconfig.me/ip', 'https://icanhazip.com') {
        try {
            $ip = (Invoke-RestMethod -Uri $svc -TimeoutSec 5 -ErrorAction Stop).ToString().Trim()
            if ($ip -match '^[0-9.]+$|^[0-9a-f:]+$') { return $ip }
        } catch { continue }
    }
    return $null
}

function Get-ShuttledBinary {
    param(
        [string]$Version = 'latest',
        [string]$Arch
    )
    $ext = '.exe'
    $base = if ($Version -eq 'latest') {
        "https://github.com/$REPO/releases/latest/download"
    } else {
        "https://github.com/$REPO/releases/download/$Version"
    }
    $url = "$base/shuttled-windows-$Arch$ext"

    if (-not (Test-Path $INSTALL_DIR)) { New-Item -ItemType Directory -Path $INSTALL_DIR | Out-Null }
    $dest = Join-Path $INSTALL_DIR 'shuttled.exe'

    Write-Info "Downloading shuttled..."
    Write-Host "  $url" -ForegroundColor DarkGray

    try {
        Invoke-WebRequest -Uri $url -OutFile $dest -UseBasicParsing -ErrorAction Stop
    } catch {
        Write-Err "Download failed: $($_.Exception.Message)"
    }

    Write-Info "Installed to $dest"
    return $dest
}
```

- [ ] **Step 3.2: Smoke (the function bodies parse cleanly)**

```powershell
. scripts/install-windows.ps1 status
```

Expected: prints status (or "service not installed"); no parse errors.

- [ ] **Step 3.3: Commit**

```bash
git add scripts/install-windows.ps1
git commit -m "feat(scripts): Windows installer platform detection + download"
```

---

## Task 4: Windows — interactive wizard

**Files:**
- Modify: `scripts/install-windows.ps1`

- [ ] **Step 4.1: Add the wizard function**

```powershell
function Invoke-Wizard {
    Write-Host ''
    Write-Host '╔══════════════════════════════════════════╗' -ForegroundColor Cyan
    Write-Host '║   Shuttle Server — Setup Wizard          ║' -ForegroundColor Cyan
    Write-Host '╚══════════════════════════════════════════╝' -ForegroundColor Cyan
    Write-Host ''

    # --- Step 1: domain or IP ---
    Write-Host 'Step 1/3 — Server Address' -ForegroundColor White
    Write-Host ''
    Write-Host '  1) Use a domain name'
    Write-Host '  2) Use server IP address'
    Write-Host ''
    do {
        $mode = Read-Host '  Choose [1/2] (default: 2)'
        if ([string]::IsNullOrWhiteSpace($mode)) { $mode = '2' }
    } until ($mode -in '1', '2')

    if ($mode -eq '1') {
        do {
            $script:domain = Read-Host '  Enter your domain'
        } until ($script:domain)
    } else {
        $detected = Get-PublicIP
        if ($detected) {
            $entered = Read-Host "  Server IP [$detected]"
            $script:domain = if ($entered) { $entered } else { $detected }
        } else {
            do {
                $script:domain = Read-Host '  Could not detect IP. Enter server IP'
            } until ($script:domain)
        }
    }
    Write-Info "Using address: $script:domain"

    # --- Step 2: password ---
    Write-Host ''
    Write-Host 'Step 2/3 — Authentication' -ForegroundColor White
    $script:password = Read-Host '  Set a password (leave empty to auto-generate)'
    if ([string]::IsNullOrWhiteSpace($script:password)) {
        $bytes = New-Object byte[] 24
        [Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)
        $script:password = [Convert]::ToBase64String($bytes).Replace('/', '').Replace('+', '').Replace('=', '').Substring(0, 16)
        Write-Info "Generated password: $script:password"
    }

    # --- Step 3: transport ---
    Write-Host ''
    Write-Host 'Step 3/3 — Transport Protocol' -ForegroundColor White
    Write-Host '  1) Both H3 + Reality (recommended)'
    Write-Host '  2) H3/QUIC only'
    Write-Host '  3) Reality only'
    do {
        $t = Read-Host '  Choose [1/2/3] (default: 1)'
        if ([string]::IsNullOrWhiteSpace($t)) { $t = '1' }
    } until ($t -in '1', '2', '3')
    $script:transport = @{ '1' = 'both'; '2' = 'h3'; '3' = 'reality' }[$t]

    Write-Host ''
    Write-Info 'Generating config...'
    & (Join-Path $INSTALL_DIR 'shuttled.exe') init `
        --dir $CONFIG_DIR `
        --domain $script:domain `
        --password $script:password `
        --transport $script:transport
}

function Invoke-AutoConfigure {
    Write-Info 'Running auto-config...'
    $args = @('init', '--dir', $CONFIG_DIR, '--transport', $Transport)
    if ($Domain)   { $args += @('--domain',   $Domain) }
    if ($Password) { $args += @('--password', $Password) }
    & (Join-Path $INSTALL_DIR 'shuttled.exe') @args
}
```

- [ ] **Step 4.2: Commit**

```bash
git add scripts/install-windows.ps1
git commit -m "feat(scripts): Windows installer wizard"
```

---

## Task 5: Windows — service registration and firewall rules

**Files:**
- Modify: `scripts/install-windows.ps1`

- [ ] **Step 5.1: Add service and firewall functions**

```powershell
function Register-ShuttledService {
    Write-Info 'Registering Windows service...'
    $exe = Join-Path $INSTALL_DIR 'shuttled.exe'
    $cfg = Join-Path $CONFIG_DIR 'server.yaml'

    # Use shuttled's own service install subcommand (existing service_windows.go).
    & $exe service install --config $cfg
    if ($LASTEXITCODE -ne 0) { Write-Err 'service install returned non-zero' }

    & $exe service start
    if ($LASTEXITCODE -ne 0) { Write-Warn 'service start returned non-zero; check Event Viewer' }
}

function Set-FirewallRules {
    Write-Host ''
    Write-Host 'shuttled needs inbound firewall rules for the transports you enabled.'
    $confirm = Read-Host 'Add firewall rules now? [Y/n]'
    if ($confirm -match '^(n|no)$') {
        Write-Warn 'Skipped firewall rule creation. You will need to add rules manually before clients can connect.'
        return
    }

    # Read ports from server.yaml
    $cfgFile = Join-Path $CONFIG_DIR 'server.yaml'
    if (-not (Test-Path $cfgFile)) {
        Write-Warn "Config not found at $cfgFile; skipping firewall."
        return
    }

    $portRegex = '(?m)^\s*listen:\s*[":\s]*(\d+)'
    $ports = (Select-String -Path $cfgFile -Pattern $portRegex).Matches | ForEach-Object {
        $_.Groups[1].Value
    } | Sort-Object -Unique

    foreach ($port in $ports) {
        $ruleName = "Shuttled-Inbound-$port"
        Write-Info "Adding firewall rule: TCP/UDP $port (rule: $ruleName)"
        New-NetFirewallRule -DisplayName $ruleName -Direction Inbound -Protocol TCP -LocalPort $port -Action Allow -ErrorAction SilentlyContinue | Out-Null
        New-NetFirewallRule -DisplayName "$ruleName-UDP" -Direction Inbound -Protocol UDP -LocalPort $port -Action Allow -ErrorAction SilentlyContinue | Out-Null
    }
}
```

- [ ] **Step 5.2: Commit**

```bash
git add scripts/install-windows.ps1
git commit -m "feat(scripts): Windows installer service registration + firewall prompt"
```

---

## Task 6: Windows — install/uninstall/upgrade/status orchestrators

**Files:**
- Modify: `scripts/install-windows.ps1`

- [ ] **Step 6.1: Add the four entry-point functions**

```powershell
function Install-Shuttled {
    $arch = Get-Architecture
    Write-Info "Platform: windows/$arch"

    Get-ShuttledBinary -Version $Version -Arch $arch | Out-Null

    if ($Auto) {
        Invoke-AutoConfigure
    } else {
        Invoke-Wizard
    }

    Register-ShuttledService
    Set-FirewallRules

    Write-Host ''
    Write-Host '╔══════════════════════════════════════════╗' -ForegroundColor Green
    Write-Host '║   Setup complete! shuttled is running.   ║' -ForegroundColor Green
    Write-Host '╚══════════════════════════════════════════╝' -ForegroundColor Green
    Write-Host ''
    Write-Host "  Manage:  Get-Service $SERVICE_NAME"
    Write-Host "  Logs:    Get-EventLog -LogName Application -Source $SERVICE_NAME -Newest 50"
    Write-Host "  Config:  $CONFIG_DIR\server.yaml"
    Write-Host "  Share:   & '$INSTALL_DIR\shuttled.exe' share -c '$CONFIG_DIR\server.yaml'"
}

function Uninstall-Shuttled {
    Write-Info 'Uninstalling shuttled...'
    $exe = Join-Path $INSTALL_DIR 'shuttled.exe'
    if (Test-Path $exe) {
        & $exe service stop      | Out-Null
        & $exe service uninstall | Out-Null
    }

    Get-NetFirewallRule -DisplayName 'Shuttled-Inbound-*' -ErrorAction SilentlyContinue | Remove-NetFirewallRule

    if (Test-Path $INSTALL_DIR) { Remove-Item -Path $INSTALL_DIR -Recurse -Force }
    Write-Info "Removed binary at $INSTALL_DIR"
    Write-Warn "Config directory $CONFIG_DIR was NOT removed (contains keys)."
    Write-Warn "To remove it: Remove-Item -Path $CONFIG_DIR -Recurse -Force"
}

function Upgrade-Shuttled {
    param([string]$To = 'latest')
    Write-Info "Upgrading to $To..."
    $exe = Join-Path $INSTALL_DIR 'shuttled.exe'
    if (Test-Path $exe) { & $exe service stop | Out-Null }
    Get-ShuttledBinary -Version $To -Arch (Get-Architecture) | Out-Null
    & $exe service start | Out-Null
    Write-Info 'Upgrade complete.'
}

function Show-Status {
    $svc = Get-Service -Name $SERVICE_NAME -ErrorAction SilentlyContinue
    if ($svc) {
        Write-Host "shuttled status: $($svc.Status)" -ForegroundColor (
            if ($svc.Status -eq 'Running') { 'Green' } else { 'Yellow' }
        )
    } else {
        Write-Warn 'shuttled service is not installed.'
    }
    if (Test-Path "$CONFIG_DIR\server.yaml") {
        Write-Info "Config: $CONFIG_DIR\server.yaml"
    }
}
```

- [ ] **Step 6.2: End-to-end lint**

```powershell
Invoke-ScriptAnalyzer -Path scripts/install-windows.ps1 -Severity Warning
```

Expected: clean (no errors, no undefined-variable warnings).

- [ ] **Step 6.3: Commit**

```bash
git add scripts/install-windows.ps1
git commit -m "feat(scripts): Windows installer install/uninstall/upgrade/status"
```

---

## Task 7: Windows installer smoke checklist

**Files:**
- Create: `scripts/install-smoke.md`

- [ ] **Step 7.1: Write the checklist**

```markdown
# Installer Smoke Checklist

Run before each release that touches the installer scripts.

## Linux (`scripts/install-linux.sh`)

On a fresh Ubuntu 22.04 VM:

- [ ] `curl -fsSL <raw-url>/scripts/install-linux.sh | sudo bash` runs the wizard.
- [ ] After wizard: `systemctl status shuttled` shows `active (running)`.
- [ ] `journalctl -u shuttled -f` shows no errors.
- [ ] `sudo bash install.sh status` reports running.
- [ ] `sudo bash install.sh upgrade <prev>` succeeds and `systemctl status` still shows running.
- [ ] `sudo bash install.sh uninstall` removes binary and service; config directory remains.

## macOS (Homebrew tap)

On macOS 14 with Homebrew installed:

- [ ] `brew tap shuttleX/shuttle` succeeds.
- [ ] `brew install shuttled` completes; binary lives at `$(brew --prefix)/bin/shuttled`.
- [ ] `shuttled --version` prints the expected version.
- [ ] `shuttled init --dir ~/.config/shuttle` generates a config.
- [ ] `brew services start shuttled` brings it up.
- [ ] `brew services list` shows `shuttled` as `started`.
- [ ] `brew services stop shuttled` cleanly stops it.
- [ ] `brew uninstall shuttled` removes binary; config remains.

## Windows (`scripts/install-windows.ps1`)

On a fresh Windows Server 2022 VM, in an elevated PowerShell session:

- [ ] `iwr -useb <raw-url>/scripts/install-windows.ps1 | iex` runs the wizard.
- [ ] After wizard: `Get-Service shuttled` shows `Running`.
- [ ] Firewall prompt appears and creates the expected rules.
- [ ] `.\install-windows.ps1 status` reports Running.
- [ ] `.\install-windows.ps1 upgrade <prev>` succeeds.
- [ ] `.\install-windows.ps1 uninstall` removes service, binary, and firewall rules.

## Notes
- Each platform should be tested with a clean OS image — installer interactions with pre-existing tools are out of scope here.
- If any step fails, file a regression test (in `scripts/test-install-windows.ps1` for Windows, in `sandbox/` for Linux) before fixing.
```

- [ ] **Step 7.2: Commit**

```bash
git add scripts/install-smoke.md
git commit -m "docs(scripts): installer smoke checklist"
```

---

## Task 8: Homebrew tap repository — create the repo

**Files:**
- (New repository, not in this codebase.)

> **Note:** This task creates a new public repo on GitHub. The plan executor must explicitly confirm with the user before running `gh repo create`, per the system convention for visible/destructive actions.

- [ ] **Step 8.1: Confirm with the user**

Show the planned command and wait for explicit approval:

```bash
gh repo create shuttleX/homebrew-shuttle --public \
    --description "Homebrew Tap for Shuttle (https://github.com/shuttleX/shuttle)"
```

- [ ] **Step 8.2: Create the repo**

After approval, run the command above.

- [ ] **Step 8.3: Clone locally**

```bash
gh repo clone shuttleX/homebrew-shuttle /tmp/homebrew-shuttle
cd /tmp/homebrew-shuttle
mkdir -p Formula .github/workflows
```

---

## Task 9: Homebrew formula — `shuttle` (CLI client)

**Files:**
- Create (in `homebrew-shuttle` repo): `Formula/shuttle.rb`

- [ ] **Step 9.1: Write the formula**

```ruby
# Formula/shuttle.rb
class Shuttle < Formula
  desc "Multi-transport network toolkit (CLI client)"
  homepage "https://github.com/shuttleX/shuttle"
  version "0.4.0"

  on_macos do
    on_arm do
      url "https://github.com/shuttleX/shuttle/releases/download/v#{version}/shuttle-darwin-arm64.tar.gz"
      sha256 "REPLACE_WITH_REAL_SHA256_FROM_CHECKSUMS_TXT"
    end
    on_intel do
      url "https://github.com/shuttleX/shuttle/releases/download/v#{version}/shuttle-darwin-amd64.tar.gz"
      sha256 "REPLACE_WITH_REAL_SHA256_FROM_CHECKSUMS_TXT"
    end
  end

  def install
    bin.install "shuttle"
    bash_completion.install "completions/shuttle.bash" => "shuttle" if File.exist?("completions/shuttle.bash")
    zsh_completion.install  "completions/_shuttle"      if File.exist?("completions/_shuttle")
    fish_completion.install "completions/shuttle.fish"  if File.exist?("completions/shuttle.fish")
  end

  test do
    assert_match(/shuttle version/i, shell_output("#{bin}/shuttle --version"))
  end
end
```

- [ ] **Step 9.2: Commit (in tap repo)**

```bash
cd /tmp/homebrew-shuttle
git add Formula/shuttle.rb
git commit -m "feat: add shuttle formula"
```

---

## Task 10: Homebrew formula — `shuttled` (server)

**Files:**
- Create (in `homebrew-shuttle` repo): `Formula/shuttled.rb`

- [ ] **Step 10.1: Write the formula**

```ruby
# Formula/shuttled.rb
class Shuttled < Formula
  desc "Multi-transport network toolkit (server)"
  homepage "https://github.com/shuttleX/shuttle"
  version "0.4.0"

  on_macos do
    on_arm do
      url "https://github.com/shuttleX/shuttle/releases/download/v#{version}/shuttled-darwin-arm64.tar.gz"
      sha256 "REPLACE_WITH_REAL_SHA256_FROM_CHECKSUMS_TXT"
    end
    on_intel do
      url "https://github.com/shuttleX/shuttle/releases/download/v#{version}/shuttled-darwin-amd64.tar.gz"
      sha256 "REPLACE_WITH_REAL_SHA256_FROM_CHECKSUMS_TXT"
    end
  end

  def install
    bin.install "shuttled"
    (etc/"shuttle").mkpath
    (etc/"shuttle").install "examples/server.example.yaml" => "server.example.yaml" if File.exist?("examples/server.example.yaml")
    bash_completion.install "completions/shuttled.bash" => "shuttled" if File.exist?("completions/shuttled.bash")
    zsh_completion.install  "completions/_shuttled"      if File.exist?("completions/_shuttled")
  end

  service do
    run [opt_bin/"shuttled", "run", "-c", "#{etc}/shuttle/server.yaml"]
    keep_alive true
    log_path     var/"log/shuttled.log"
    error_log_path var/"log/shuttled.err.log"
  end

  test do
    assert_match(/shuttled version/i, shell_output("#{bin}/shuttled --version"))
  end
end
```

- [ ] **Step 10.2: Commit (in tap repo)**

```bash
cd /tmp/homebrew-shuttle
git add Formula/shuttled.rb
git commit -m "feat: add shuttled formula"
```

---

## Task 11: Homebrew tap README and CI

**Files:**
- Create (in `homebrew-shuttle` repo): `README.md`, `.github/workflows/test.yml`

- [ ] **Step 11.1: Tap README**

```markdown
# Homebrew Tap for Shuttle

Official Homebrew tap for [Shuttle](https://github.com/shuttleX/shuttle), a multi-transport network toolkit.

## Install

```bash
brew tap shuttleX/shuttle
brew install shuttle    # CLI client
brew install shuttled   # server daemon
```

## Run shuttled as a service

```bash
shuttled init                        # generate config
brew services start shuttled         # launchd service
```

Logs land at `$(brew --prefix)/var/log/shuttled.log` (and `.err.log`).

## Updating

```bash
brew update
brew upgrade shuttle shuttled
```

This tap is auto-PR'd from the main `shuttle` repo on each release. If you spot a stale formula, please file an issue here.
```

- [ ] **Step 11.2: PR test workflow**

```yaml
# .github/workflows/test.yml
name: Test formulae
on: [push, pull_request]

jobs:
  test:
    runs-on: macos-latest
    steps:
      - uses: actions/checkout@v4
      - run: brew test-bot --only-tap-syntax
      - run: brew test-bot --only-formulae
```

- [ ] **Step 11.3: Commit and push**

```bash
cd /tmp/homebrew-shuttle
git add README.md .github/workflows/test.yml
git commit -m "feat: README + tap CI"
git push origin main
```

---

## Task 12: Wire formula bumps into release.yml

**Files:**
- Modify: `.github/workflows/release.yml` (in the main shuttle repo)

- [ ] **Step 12.1: Add the bump step**

Locate the job that publishes release artifacts. Append a step that runs **after** all platform binaries are uploaded:

```yaml
      - name: Bump Homebrew tap formulae
        if: startsWith(github.ref, 'refs/tags/v')
        uses: mislav/bump-homebrew-formula-action@v3
        with:
          formula-name: shuttle
          formula-path: Formula/shuttle.rb
          homebrew-tap: shuttleX/homebrew-shuttle
          base-branch: main
          download-url: https://github.com/shuttleX/shuttle/releases/download/${{ github.ref_name }}/shuttle-darwin-amd64.tar.gz
        env:
          COMMITTER_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}

      - name: Bump Homebrew tap formulae (shuttled)
        if: startsWith(github.ref, 'refs/tags/v')
        uses: mislav/bump-homebrew-formula-action@v3
        with:
          formula-name: shuttled
          formula-path: Formula/shuttled.rb
          homebrew-tap: shuttleX/homebrew-shuttle
          base-branch: main
          download-url: https://github.com/shuttleX/shuttle/releases/download/${{ github.ref_name }}/shuttled-darwin-amd64.tar.gz
        env:
          COMMITTER_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}
```

- [ ] **Step 12.2: Document the required secret**

Add a note in `docs/release.md` (create if missing):

```markdown
## Required GitHub Secrets

- `HOMEBREW_TAP_TOKEN` — a fine-grained PAT with `contents:write` permission on `shuttleX/homebrew-shuttle`. Set in repo Settings → Secrets and variables → Actions.
```

- [ ] **Step 12.3: Commit**

```bash
git add .github/workflows/release.yml docs/release.md
git commit -m "ci(release): auto-bump Homebrew tap formulae on tag"
```

---

## Task 13: README install section reorganisation

**Files:**
- Modify: `README.md` (install section)

- [ ] **Step 13.1: Add decision table**

Replace the current install section (or create one if absent) with:

```markdown
## Install

### Which installer do I want?

| If you want to... | Use |
|---|---|
| Run the **server** (`shuttled`) on a VPS | CLI installer (Linux / macOS / Windows below) |
| Run a **desktop client** with a UI | GUI installer (`.dmg` / `.exe` / AppImage from [Releases](https://github.com/shuttleX/shuttle/releases)) |
| Automate / script / CI | CLI installer |

> **Read [SECURITY.md](./SECURITY.md) before deploying to production.**

### Linux

```bash
curl -fsSL https://raw.githubusercontent.com/shuttleX/shuttle/main/scripts/install-linux.sh | sudo bash
```

Or with environment variables for non-interactive setup:

```bash
sudo SHUTTLE_DOMAIN=proxy.example.com SHUTTLE_PASSWORD=secret \
    bash -c "$(curl -fsSL https://raw.githubusercontent.com/shuttleX/shuttle/main/scripts/install-linux.sh) install --auto"
```

### macOS (Homebrew)

```bash
brew tap shuttleX/shuttle
brew install shuttled
shuttled init
brew services start shuttled
```

### Windows (PowerShell, run as Administrator)

```powershell
iwr -useb https://raw.githubusercontent.com/shuttleX/shuttle/main/scripts/install-windows.ps1 | iex
```

### Verify

After install, on any platform:

```bash
shuttled --version
curl http://127.0.0.1:8443/api/health/ready    # 200 OK once listeners are bound
```
```

- [ ] **Step 13.2: Commit**

```bash
git add README.md
git commit -m "docs(readme): reorganise install section with decision table"
```

---

## Task 14: Per-platform deploy guides

**Files:**
- Create: `docs/site/en/install/linux.md`, `docs/site/en/install/macos.md`, `docs/site/en/install/windows.md`
- Create: `docs/site/zh/install/{linux,macos,windows}.md`

- [ ] **Step 14.1: Linux guide**

```markdown
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
- [Configuration reference](../configuration.md).
```

- [ ] **Step 14.2: macOS guide**

```markdown
# macOS Install

For headless macOS deployments. For a desktop client, install the `.dmg` from the [Releases page](https://github.com/shuttleX/shuttle/releases).

## Prerequisites
- macOS 12 or later
- [Homebrew](https://brew.sh)

## Install

```bash
brew tap shuttleX/shuttle
brew install shuttled
```

## Configure and start

```bash
shuttled init
brew services start shuttled
```

`brew services` registers a launchd plist. Logs:

```bash
tail -f $(brew --prefix)/var/log/shuttled.log
```

## Upgrade and uninstall

```bash
brew update && brew upgrade shuttled
brew services stop shuttled
brew uninstall shuttled
```

The config in `$(brew --prefix)/etc/shuttle/` is preserved across upgrades.

## Read next
- [SECURITY.md](https://github.com/shuttleX/shuttle/blob/main/SECURITY.md)
```

- [ ] **Step 14.3: Windows guide**

```markdown
# Windows Install

For headless Windows Server deployments. For a desktop client, install the `.exe` from the [Releases page](https://github.com/shuttleX/shuttle/releases).

## Prerequisites
- Windows Server 2019/2022 or Windows 10/11
- PowerShell 5.1+ or 7+
- An elevated PowerShell session (Run as Administrator)

## Install

```powershell
iwr -useb https://raw.githubusercontent.com/shuttleX/shuttle/main/scripts/install-windows.ps1 | iex
```

If your PowerShell execution policy blocks the script, temporarily relax it:

```powershell
Set-ExecutionPolicy -ExecutionPolicy Bypass -Scope Process -Force
```

The wizard will detect architecture, download the binary, run the same three-step setup as Linux, and register a Windows service. It will prompt before adding firewall rules.

## Manage

```powershell
Get-Service shuttled
Restart-Service shuttled
Get-EventLog -LogName Application -Source shuttled -Newest 50
```

## Upgrade and uninstall

```powershell
.\install-windows.ps1 upgrade v0.4.1
.\install-windows.ps1 uninstall
```

## Read next
- [SECURITY.md](https://github.com/shuttleX/shuttle/blob/main/SECURITY.md)
```

- [ ] **Step 14.4: Translate to Chinese**

For each of the three guides, create `docs/site/zh/install/<os>.md` with a Chinese translation of the same content. Use the existing translation conventions in `docs/site/zh/`.

- [ ] **Step 14.5: Commit**

```bash
git add docs/site/en/install/ docs/site/zh/install/
git commit -m "docs(site): per-platform install guides (en + zh)"
```

---

## Task 15: End-to-end verification

- [ ] **Step 15.1: Lint Windows installer**

```powershell
Invoke-ScriptAnalyzer -Path scripts/install-windows.ps1 -Severity Warning
```

Expected: clean.

- [ ] **Step 15.2: Lint Linux wrapper**

```bash
shellcheck scripts/install-linux.sh
```

Expected: clean.

- [ ] **Step 15.3: Manual smoke per `scripts/install-smoke.md`**

Run through the Linux section on a clean VM. (macOS and Windows can be deferred to release time, since the tap repo and bump action only activate on tag.)

- [ ] **Step 15.4: Final repo state check**

```bash
git status        # clean
git log --oneline -10
```

---

## Self-Review Notes

- Tap repo creation is gated behind explicit user confirmation — the plan executor must not run `gh repo create` without showing the command first.
- The `HOMEBREW_TAP_TOKEN` secret is documented but cannot be created automatically; a human must add it once.
- Windows firewall is interactive by design (per Section 5 design review) — a silent install via `--Auto` skips the prompt and prints a warning instead.
- The `gui` (desktop) installers are intentionally out of scope: NSIS for Windows and the existing `.app` bundle for macOS continue to ship via `release.yml`. The decision table in README disambiguates.
- All file paths reference real existing files except those in the tap repo (which is created in Task 8).
