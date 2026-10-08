# slackware-cli-manager

`slackware-cli-manager` is a terminal UI for common Slackware administration tasks: package maintenance, SlackBuild workflows, system inspection, services, backups, disks, network state, and configuration editing.

The app now supports both:

- read-only startup for inspection and navigation
- root-gated mutation for actions that change the system

## Current Scope

Primary tabs:

- `F1` Update
- `F2` sbotools
- `F3` Users
- `F4` Mirrors
- `F5` Search
- `F6` Config

Secondary tabs:

- `F7` SysInfo
- `F8` Services
- `F9` Packages
- `F10` Backup
- `F11` Network
- `F12` Logs

Additional tabs:

- `Ctrl+K` Kernel
- `Ctrl+J` Cron
- `Ctrl+D` Disks
- `Ctrl+S` Settings

## Requirements

- Slackware Linux
- Go `1.24+` only if you build from source
- An interactive terminal at least 80 columns by 24 rows
- root privileges only for mutating actions

Supported Slackware versions are detected from `/etc/slackware-version`. The codebase currently targets common Slackware `14.x`, `15.0`, and `-current` layouts.

## Installation

### Prebuilt release assets

Slackware users do not need a Go toolchain.

Download release assets from the project's GitHub Releases page:

- `slackware-cli-manager-<version>-x86_64-1.txz`
- `slackware-cli-manager-v<version>-x86_64-linux.tar.gz`
- `SHA256SUMS.txt`

GitHub releases publish two installable asset types for tagged versions:

- a portable static binary archive for `x86_64`
- a Slackware package: `slackware-cli-manager-<version>-x86_64-1.txz`

Recommended for Slackware: install the package with `upgradepkg`.

Verify the download:

```bash
sha256sum -c SHA256SUMS.txt
```

Install or upgrade the Slackware package:

```bash
sudo upgradepkg --install-new slackware-cli-manager-0.1.1-x86_64-1.txz
```

Portable archive install without package management:

```bash
tar -xzf slackware-cli-manager-v0.1.1-x86_64-linux.tar.gz
sudo install -m 0755 slackware-cli-manager-v0.1.1-x86_64-linux/slackware-cli-manager /usr/local/bin/slackware-cli-manager
```

Remove a package install:

```bash
sudo removepkg slackware-cli-manager
```

Remove a portable install:

```bash
sudo rm -f /usr/local/bin/slackware-cli-manager
```

### With `go install`

```bash
go install github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/cmd/slackware-cli-manager@latest
```

### From source

```bash
git clone https://github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager
cd Gosh-Slack-CLI-Manager
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o slackware-cli-manager ./cmd/slackware-cli-manager
sudo install -m 0755 slackware-cli-manager /usr/local/bin/slackware-cli-manager
```

## Usage

Read-only mode:

```bash
slackware-cli-manager
```

Root mode for system changes:

```bash
sudo slackware-cli-manager
```

The header shows when the app is running in read-only mode. Actions that require privileges are blocked centrally and report the reason in the UI instead of failing later in a component.

Use `Alt+Left` / `Alt+Right` to move between all tabs. `F5` refreshes several secondary views; use `Alt+Left` / `Alt+Right` to reach Search from those views. `Ctrl+Q` exits the application, except while editing a configuration file, where it closes the editor or warns about unsaved changes. In the editor, `Ctrl+S` saves and `Ctrl+X` discards changes; `Ctrl+C` exits the application.

User preferences are saved to `$XDG_CONFIG_HOME/slackware-cli-manager/config.toml` when `XDG_CONFIG_HOME` is an absolute path, otherwise `~/.config/slackware-cli-manager/config.toml`; root preferences are saved to `/etc/slackware-cli-manager/config.toml`. In Settings, use `Tab` to change sections, arrows to select and change options, `s` to save, and `r` to reset the form to defaults.

The development build and tests also run on other Linux distributions. On a non-Slackware host the UI warns that Slackware detection failed and uses its Current fallback. This does not provide Slackware package tools, service scripts, or system configuration. Verify administration workflows on a disposable Slackware system with the relevant tools installed.

## Behavior Notes

- Long-running package and system tasks execute in the background so the UI stays responsive.
- Command output is streamed back into the active workflow instead of blocking the draw loop.
- System file writes use atomic replacement for supported config paths.
- Mirror changes validate that exactly one active mirror remains configured.

## Release Notes

Current release notes and milestone summaries live in [CHANGELOG.md](CHANGELOG.md).

## Development

```bash
gofmt -l .
go vet ./...
go test ./...
```

The binary is a single static executable with no cgo. The terminal layer is a small cell-buffer toolkit in `internal/tui` (layout solver, blocks, paragraphs, lists, gauges, text editor) on top of [tcell](https://github.com/gdamore/tcell).

CI runs the same checks on pushes to `main` and on pull requests.

## License

MIT. See [LICENSE](LICENSE).
