#!/usr/bin/env bash
# Cross-compile Nudge for Windows 10/11 x64 and package it as a zip.
#
# Usage: scripts/build-windows.sh [version]
#
# Output (nothing else is written to the repository):
#   dist/nudge-<version>-windows-x64.zip
#   dist/nudge-<version>-windows-x64.zip.sha256
#
# The zip is flat (extract it anywhere and double-click "Install Nudge.cmd"):
#   nudge.exe              console build: `nudge pair ...` CLI, or the server by hand
#   nudge-background.exe   GUI-subsystem build (no console window), used for autostart
#   nudge.ico              multi-size icon (16..256) rendered from pkgbuild/nudge.svg
#   Install Nudge.cmd      double-click launcher for install.ps1
#   Uninstall Nudge.cmd    double-click launcher for uninstall.ps1
#   install.ps1 uninstall.ps1 toggle-autostart.ps1 firewall.ps1
#   README-windows.txt
#
# Needs: go (1.24), rsvg-convert, magick (ImageMagick 7), zip, sha256sum.
# Text files are converted to CRLF and the .ps1/.txt files get a UTF-8 BOM in
# the zip, because Windows PowerShell 5.1 reads BOM-less scripts as ANSI and
# would garble the Spanish messages.
set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo"

# Version: argument, else a Go `version`/`Version` constant or variable in
# cmd/server, else 0.1.0.
detect_version() {
  local v
  v="$(grep -hoE '^[[:space:]]*(const|var)?[[:space:]]*[vV]ersion[[:space:]]*(string)?[[:space:]]*=[[:space:]]*"[0-9][^"]*"' cmd/server/*.go 2>/dev/null \
    | head -n1 | sed -E 's/.*"([^"]+)".*/\1/' || true)"
  printf '%s\n' "${v:-0.1.0}"
}
version="${1:-$(detect_version)}"
if [[ ! "$version" =~ ^[0-9A-Za-z][0-9A-Za-z.+_-]*$ ]]; then
  echo "build-windows: invalid version '$version'" >&2
  exit 2
fi

# Use the mise Go install only when no working go is on PATH (a mise shim
# without a configured version is on PATH but fails).
if ! go version >/dev/null 2>&1; then
  export PATH="$HOME/.local/share/mise/installs/go/1.24.13/bin:$PATH"
fi
for tool in go rsvg-convert magick zip sha256sum; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "build-windows: missing required tool: $tool" >&2
    exit 1
  fi
done

src_dir="packaging/windows"
svg="pkgbuild/nudge.svg"
name="nudge-${version}-windows-x64.zip"
text_files=(
  "Install Nudge.cmd"
  "Uninstall Nudge.cmd"
  "install.ps1"
  "uninstall.ps1"
  "toggle-autostart.ps1"
  "firewall.ps1"
  "README-windows.txt"
)
for f in "${text_files[@]}"; do
  [[ -f "$src_dir/$f" ]] || { echo "build-windows: missing $src_dir/$f" >&2; exit 1; }
done
[[ -f "$svg" ]] || { echo "build-windows: missing $svg" >&2; exit 1; }

# Everything is staged in a temp dir that is always removed.
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
pkg="$stage/pkg"
mkdir -p "$pkg" "$stage/icon"

echo "==> go build (windows/amd64, version $version)"
export CGO_ENABLED=0 GOOS=windows GOARCH=amd64
go build -trimpath -ldflags "-s -w" -o "$pkg/nudge.exe" ./cmd/server
go build -trimpath -ldflags "-s -w -H=windowsgui" -o "$pkg/nudge-background.exe" ./cmd/server
unset CGO_ENABLED GOOS GOARCH

echo "==> nudge.ico"
sizes=(16 24 32 48 64 128 256)
pngs=()
for s in "${sizes[@]}"; do
  rsvg-convert -w "$s" -h "$s" -o "$stage/icon/$s.png" "$svg"
  pngs+=("$stage/icon/$s.png")
done
magick "${pngs[@]}" "$pkg/nudge.ico"

echo "==> scripts and docs"
bom=$'\xEF\xBB\xBF'
for f in "${text_files[@]}"; do
  out="$pkg/$f"
  cp "$src_dir/$f" "$out"
  # LF -> CRLF (idempotent if the source already has CRLF).
  sed -i 's/\r$//; s/$/\r/' "$out"
  case "$f" in
    *.ps1|*.txt)
      if [[ "$(head -c 3 "$out")" != "$bom" ]]; then
        { printf '%s' "$bom"; cat "$out"; } > "$out.tmp"
        mv "$out.tmp" "$out"
      fi
      ;;
    *.cmd)
      # cmd.exe uses the OEM code page: keep launchers pure ASCII.
      if LC_ALL=C grep -q '[^[:print:][:space:]]' "$out"; then
        echo "build-windows: $f must be ASCII" >&2
        exit 1
      fi
      ;;
  esac
done

echo "==> zip"
(
  cd "$pkg"
  zip -X -9 -q "$stage/$name" nudge.exe nudge-background.exe nudge.ico "${text_files[@]}"
)
(
  cd "$stage"
  sha256sum "$name" > "$name.sha256"
)
mkdir -p dist
mv -f "$stage/$name" "dist/$name"
mv -f "$stage/$name.sha256" "dist/$name.sha256"

echo "==> dist/$name"
unzip -l "dist/$name"
ls -l "dist/$name" "dist/$name.sha256"
cat "dist/$name.sha256"
