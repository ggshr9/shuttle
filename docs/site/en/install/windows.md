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
