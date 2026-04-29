# macOS Install

For headless macOS deployments. For a desktop client, install the `.dmg` from the [Releases page](https://github.com/ggshr9/shuttle/releases).

## Prerequisites
- macOS 12 or later
- [Homebrew](https://brew.sh)

## Install

```bash
brew tap ggshr9/shuttle
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
- [SECURITY.md](https://github.com/ggshr9/shuttle/blob/main/SECURITY.md)
