# Windows 安装

适用于无界面（headless）的 Windows Server 部署。如需桌面客户端，请前往 [Releases 页面](https://github.com/shuttleX/shuttle/releases) 下载 `.exe`。

## 前置条件
- Windows Server 2019/2022 或 Windows 10/11
- PowerShell 5.1+ 或 7+
- 以管理员身份运行的 PowerShell 会话

## 安装

```powershell
iwr -useb https://raw.githubusercontent.com/shuttleX/shuttle/main/scripts/install-windows.ps1 | iex
```

如果 PowerShell 的执行策略阻止脚本运行，可临时放宽：

```powershell
Set-ExecutionPolicy -ExecutionPolicy Bypass -Scope Process -Force
```

向导会自动检测架构、下载二进制，并执行与 Linux 相同的三步配置流程，最后注册为 Windows 服务。添加防火墙规则前会先征求确认。

## 管理

```powershell
Get-Service shuttled
Restart-Service shuttled
Get-EventLog -LogName Application -Source shuttled -Newest 50
```

## 升级与卸载

```powershell
.\install-windows.ps1 upgrade v0.4.1
.\install-windows.ps1 uninstall
```

## 延伸阅读
- [SECURITY.md](https://github.com/shuttleX/shuttle/blob/main/SECURITY.md)
