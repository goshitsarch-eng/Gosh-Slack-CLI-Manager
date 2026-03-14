#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "usage: $0 <tag-or-version>" >&2
  exit 1
fi

VERSION="${1#v}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

awk -v version="$VERSION" '
  $0 ~ "^## \\[" version "\\]" { capture = 1; next }
  capture && $0 ~ "^## \\[" { exit }
  capture { print }
' "$ROOT_DIR/CHANGELOG.md"
