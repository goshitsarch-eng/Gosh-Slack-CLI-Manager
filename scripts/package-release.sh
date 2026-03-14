#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 3 ]; then
  echo "usage: $0 <binary-path> <tag-or-version> <output-dir>" >&2
  exit 1
fi

APP_NAME="slackware-cli-manager"
BINARY_PATH="$1"
RAW_VERSION="$2"
OUTPUT_DIR="$3"
VERSION="${RAW_VERSION#v}"
TARGET_TRIPLE="x86_64-unknown-linux-musl"
PORTABLE_BASENAME="${APP_NAME}-${RAW_VERSION}-${TARGET_TRIPLE}"
SLACKWARE_PKGNAME="${APP_NAME}-${VERSION}-x86_64-1"

if [ ! -x "$BINARY_PATH" ]; then
  echo "binary not found or not executable: $BINARY_PATH" >&2
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

PORTABLE_ROOT="$TMP_DIR/$PORTABLE_BASENAME"
PKG_ROOT="$TMP_DIR/$SLACKWARE_PKGNAME"
DOC_ROOT="$PKG_ROOT/usr/doc/$SLACKWARE_PKGNAME"
INSTALL_ROOT="$PKG_ROOT/install"
DIST_DIR="$OUTPUT_DIR"

mkdir -p "$DIST_DIR" "$PORTABLE_ROOT" "$DOC_ROOT" "$INSTALL_ROOT"

cat > "$PORTABLE_ROOT/INSTALL.txt" <<'EOF'
Portable install:

1. Extract this archive.
2. Copy `slackware-cli-manager` somewhere on your PATH, for example:

   install -m 0755 slackware-cli-manager /usr/local/bin/slackware-cli-manager

3. Run it as your user for read-only inspection, or with sudo for mutating actions.
EOF

install -m 0755 "$BINARY_PATH" "$PORTABLE_ROOT/$APP_NAME"
install -m 0644 "$ROOT_DIR/README.md" "$PORTABLE_ROOT/README.md"
install -m 0644 "$ROOT_DIR/CHANGELOG.md" "$PORTABLE_ROOT/CHANGELOG.md"
install -m 0644 "$ROOT_DIR/LICENSE" "$PORTABLE_ROOT/LICENSE"

tar \
  --sort=name \
  --owner=0 \
  --group=0 \
  --numeric-owner \
  -C "$TMP_DIR" \
  -czf "$DIST_DIR/${PORTABLE_BASENAME}.tar.gz" \
  "$PORTABLE_BASENAME"

install -d "$PKG_ROOT/usr/bin"
install -m 0755 "$BINARY_PATH" "$PKG_ROOT/usr/bin/$APP_NAME"
install -m 0644 "$ROOT_DIR/README.md" "$DOC_ROOT/README.md"
install -m 0644 "$ROOT_DIR/CHANGELOG.md" "$DOC_ROOT/CHANGELOG.md"
install -m 0644 "$ROOT_DIR/LICENSE" "$DOC_ROOT/LICENSE"

cat > "$INSTALL_ROOT/slack-desc" <<'EOF'
slackware-cli-manager: slackware-cli-manager (Slackware administration TUI)
slackware-cli-manager:
slackware-cli-manager: slackware-cli-manager is a terminal UI for common Slackware
slackware-cli-manager: administration tasks including updates, SlackBuild workflows,
slackware-cli-manager: package inspection, services, logs, networking, backups,
slackware-cli-manager: disks, cron inspection, kernel management, and config edits.
slackware-cli-manager:
slackware-cli-manager: The release package ships a prebuilt static binary so users
slackware-cli-manager: do not need Rust or crates.io just to install the tool.
slackware-cli-manager:
slackware-cli-manager: Homepage: https://github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager
EOF

tar \
  --sort=name \
  --owner=0 \
  --group=0 \
  --numeric-owner \
  -C "$PKG_ROOT" \
  -cJf "$DIST_DIR/${SLACKWARE_PKGNAME}.txz" \
  .

(
  cd "$DIST_DIR"
  sha256sum \
    "${PORTABLE_BASENAME}.tar.gz" \
    "${SLACKWARE_PKGNAME}.txz" \
    > SHA256SUMS.txt
)
