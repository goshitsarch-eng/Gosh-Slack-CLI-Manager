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
- Rust `1.70+` to build from source
- root privileges only for mutating actions

Supported Slackware versions are detected from `/etc/slackware-version`. The codebase currently targets common Slackware `14.x`, `15.0`, and `-current` layouts.

## Installation

### Prebuilt release assets

GitHub releases now publish two installable assets for tagged versions:

- a portable static binary archive for `x86_64`
- a Slackware package: `slackware-cli-manager-<version>-x86_64-1.txz`

Slackware package install:

```bash
sudo upgradepkg --install-new slackware-cli-manager-0.1.1-x86_64-1.txz
```

Portable archive install:

```bash
tar -xzf slackware-cli-manager-v0.1.1-x86_64-unknown-linux-musl.tar.gz
sudo install -m 0755 slackware-cli-manager-v0.1.1-x86_64-unknown-linux-musl/slackware-cli-manager /usr/local/bin/slackware-cli-manager
```

### From crates.io

```bash
cargo install --locked slackware-cli-manager
```

### From source

```bash
git clone https://github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager
cd Gosh-Slack-CLI-Manager
cargo build --release --locked
sudo install -m 0755 target/release/slackware-cli-manager /usr/local/bin/slackware-cli-manager
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

## Behavior Notes

- Long-running package and system tasks execute in the background so the UI stays responsive.
- Command output is streamed back into the active workflow instead of blocking the draw loop.
- System file writes use atomic replacement for supported config paths.
- Mirror changes validate that exactly one active mirror remains configured.

## Release Notes

Current release notes and milestone summaries live in [CHANGELOG.md](CHANGELOG.md).

## Development

```bash
cargo fmt --check
cargo clippy --all-targets --all-features -- -D warnings
cargo test --all-targets --all-features
```

CI runs the same checks on pushes to `main` and on pull requests.

## License

MIT. See [LICENSE](LICENSE).
