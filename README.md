# slackware-cli-manager

A Terminal User Interface (TUI) application for managing Slackware Linux systems. Provides an easy-to-use interface for system updates, package management, user creation, and configuration.

## Features

- **System Update** - Full system update via slackpkg (update, install-new, upgrade-all, clean-system, lilo)
- **sbotools Installer** - Automated installation of sbopkg and sbotools for SlackBuilds.org packages
- **User Setup** - Create new users with proper groups, set passwords, change default runlevel
- **Mirror Configuration** - View and select package mirrors with automatic version filtering
- **Package Search** - Search and install packages from SlackBuilds.org
- **Config Editor** - Edit slackpkg.conf, sbotools.conf, and mirrors files

## Requirements

- Slackware Linux (15.0, 14.2, or -current)
- Root privileges
- Rust 1.70+ (for building from source)

## Installation

### From crates.io

```bash
cargo install slackware-cli-manager
```

### From source

```bash
git clone https://github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager
cd Gosh-Slack-CLI-Manager
cargo build --release
sudo cp target/release/slackware-cli-manager /usr/local/bin/
```

## Usage

Run as root:

```bash
sudo slackware-cli-manager
```

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| F1-F6 | Switch to tab 1-6 |
| Alt+Left/Right | Previous/Next tab |
| Ctrl+Q | Quit |
| Tab | Next field (in forms) |
| Enter | Execute/Select |
| Up/Down | Navigate lists |

### Tabs

1. **System Update (F1)** - Run slackpkg update cycle
2. **sbotools (F2)** - Install SlackBuilds.org tools
3. **User Setup (F3)** - Create new users with groups
4. **Mirrors (F4)** - Configure package mirrors
5. **Packages (F5)** - Search SlackBuilds packages
6. **Config (F6)** - Edit configuration files

## Supported Slackware Versions

The application auto-detects your Slackware version from `/etc/slackware-version`:

- Slackware64 15.0
- Slackware64 14.2
- Slackware64 14.1
- Slackware64 -current

## Building

```bash
# Debug build
cargo build

# Release build (optimized)
cargo build --release

# Run tests
cargo test

# Check for issues
cargo clippy
```

## License

MIT License - see [LICENSE](LICENSE) for details.

## Author

goshitsarch-eng

## Contributing

Contributions are welcome! Please feel free to submit issues and pull requests.
