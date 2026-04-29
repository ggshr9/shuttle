# macOS 安装

适用于无界面（headless）的 macOS 部署。如需桌面客户端，请前往 [Releases 页面](https://github.com/ggshr9/shuttle/releases) 下载 `.dmg`。

## 前置条件
- macOS 12 或更高版本
- [Homebrew](https://brew.sh)

## 安装

```bash
brew tap ggshr9/shuttle
brew install shuttled
```

## 配置并启动

```bash
shuttled init
brew services start shuttled
```

`brew services` 会注册一个 launchd plist。日志：

```bash
tail -f $(brew --prefix)/var/log/shuttled.log
```

## 升级与卸载

```bash
brew update && brew upgrade shuttled
brew services stop shuttled
brew uninstall shuttled
```

`$(brew --prefix)/etc/shuttle/` 中的配置会在升级过程中保留。

## 延伸阅读
- [SECURITY.md](https://github.com/ggshr9/shuttle/blob/main/SECURITY.md)
