# Linux 安装

本指南介绍如何在 Linux VPS 上安装 `shuttled`（服务端守护进程）。

桌面 GUI 请前往 [Releases 页面](https://github.com/ggshr9/shuttle/releases) 下载。

## 前置条件
- Linux（Debian、Ubuntu、RHEL、Alpine、OpenWrt 等）
- root 权限（`sudo`）
- 公网 IP 或指向服务器的域名

## 一键安装

```bash
curl -fsSL https://raw.githubusercontent.com/ggshr9/shuttle/main/scripts/install-linux.sh | sudo bash
```

安装向导会：
1. 检测 CPU 架构并下载对应的二进制文件。
2. 检测公网 IP，或允许你指定域名。
3. 生成随机强密码（也可使用你指定的密码）。
4. 选择传输协议（H3、Reality 或两者）。
5. 注册经过加固的 systemd 服务并启动。

## 非交互式安装

```bash
SHUTTLE_DOMAIN=proxy.example.com \
SHUTTLE_PASSWORD=$(openssl rand -base64 32) \
SHUTTLE_TRANSPORT=both \
sudo bash -c "$(curl -fsSL https://raw.githubusercontent.com/ggshr9/shuttle/main/scripts/install-linux.sh) install --auto"
```

## 管理

```bash
systemctl status  shuttled     # 查看状态
systemctl restart shuttled     # 修改配置后重启
journalctl -u     shuttled -f  # 实时日志
```

## 升级与卸载

```bash
sudo bash install.sh upgrade v0.4.1
sudo bash install.sh uninstall
```

## 延伸阅读
- [SECURITY.md](https://github.com/ggshr9/shuttle/blob/main/SECURITY.md) — 部署前安全检查清单。
