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
