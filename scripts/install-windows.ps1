#Requires -Version 5.1
<#
.SYNOPSIS
  Shuttle Server installer for Windows.
.DESCRIPTION
  Mirrors deploy/install.sh: download → wizard → register service → firewall.
  Subcommands: install (default), uninstall, upgrade [version], status.
.EXAMPLE
  iwr -useb https://raw.githubusercontent.com/ggshr9/shuttle/main/scripts/install-windows.ps1 | iex
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

$REPO         = 'ggshr9/shuttle'
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

# --- main dispatch ---
switch ($Action) {
    'install'   { Assert-Admin; Install-Shuttled }
    'uninstall' { Assert-Admin; Uninstall-Shuttled }
    'upgrade'   { Assert-Admin; Upgrade-Shuttled $Version }
    'status'    { Show-Status }
}
