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
