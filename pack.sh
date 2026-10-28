#!/usr/bin/env bash
# pack.sh — assemble the portable XEHN Linux package (v2rayN-style layout).
#
#   ./pack.sh [-Version <tag>] [-BinPath <path>]
#
# Layout produced in dist/:
#   XEHN/
#   ├── xehn              ← the only program file
#   ├── core/
#   │   └── mihomo         ← the Clash core (downloaded on first connect)
#   ├── licenses/
#   │   └── GPL-3.0.txt
#   ├── THIRD_PARTY_NOTICES.md
#   └── README.txt
#
# Outputs: dist/XEHN.<version>.tar.gz and dist/XEHN.<version>.zip
set -euo pipefail
VERSION="${Version:-}"
BIN_PATH="${BinPath:-}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DIST="$SCRIPT_DIR/dist"
STAGE="$DIST/build"
ROOT="$DIST/XEHN"
# Resolve version
if [ -z "$VERSION" ]; then
  VERSION="0.0.0-$(date +%Y%m%d%H%M%S)"
fi
# Resolve binary path
if [ -z "$BIN_PATH" ]; then
  BIN_PATH="$SCRIPT_DIR/xehn-linux"
fi
if [ ! -f "$BIN_PATH" ]; then
  echo "error: binary not found at $BIN_PATH" >&2
  echo "build it first: go build -o xehn-linux ." >&2
  exit 1
fi
# Fresh staging tree
rm -rf "$DIST"
mkdir -p "$STAGE" "$ROOT/core" "$ROOT/licenses"
# Copy the binary as the sole program file
cp "$BIN_PATH" "$ROOT/xehn"
chmod +x "$ROOT/xehn"
echo "bundled: xehn ($(du -h "$ROOT/xehn" | cut -f1))"
# Copy README
if [ -f "$SCRIPT_DIR/README-portable.txt" ]; then
  cp "$SCRIPT_DIR/README-portable.txt" "$ROOT/README.txt"
fi
# Copy third-party notices
if [ -f "$SCRIPT_DIR/THIRD_PARTY_NOTICES.md" ]; then
  cp "$SCRIPT_DIR/THIRD_PARTY_NOTICES.md" "$ROOT/THIRD_PARTY_NOTICES.md"
fi
# Copy GPL-3.0 license
if [ -f "$SCRIPT_DIR/third_party/GPL-3.0.txt" ]; then
  cp "$SCRIPT_DIR/third_party/GPL-3.0.txt" "$ROOT/licenses/GPL-3.0.txt"
  echo "bundled: licenses/GPL-3.0.txt"
else
  echo "WARNING: third_party/GPL-3.0.txt missing" >&2
fi
# Verify no credentials/runtime state leaked
FORBIDDEN=(
  warpscout-account.json state.json settings.json cache.json
  config.yaml cache.db xehn.log
)
LEAKED=$(find "$ROOT" -type f \( -name 'warpscout-account.json' -o -name 'state.json' -o -name 'settings.json' -o -name 'cache.json' -o -name 'config.yaml' -o -name 'cache.db' -o -name 'xehn.log' -o -name 'vpngate-*.json' -o -name 'warp-report-*.txt' \) 2>/dev/null || true)
if [ -n "$LEAKED" ]; then
  echo "refusing to package credentials/runtime state: $LEAKED" >&2
  exit 1
fi
# Verify exactly one top-level program
TOP_EXE_COUNT=$(find "$ROOT" -maxdepth 1 -type f -executable | wc -l)
if [ "$TOP_EXE_COUNT" -ne 1 ]; then
  echo "release must expose exactly one top-level program, found $TOP_EXE_COUNT" >&2
  exit 1
fi
# Produce tar.gz
TAR_PATH="$DIST/XEHN.$VERSION.tar.gz"
tar -C "$DIST" -czf "$TAR_PATH" "XEHN"
echo "TAR ready: $TAR_PATH ($(du -h "$TAR_PATH" | cut -f1))"
# Produce zip
ZIP_PATH="$DIST/XEHN.$VERSION.zip"
( cd "$DIST" && zip -qr "$ZIP_PATH" "XEHN" )
echo "ZIP ready: $ZIP_PATH ($(du -h "$ZIP_PATH" | cut -f1))"
echo ""
echo "Package contents:"
find "$ROOT" -type f | sed "s|$ROOT/||" | while read f; do
  echo "  $f  ($(du -h "$ROOT/$f" | cut -f1))"
done
