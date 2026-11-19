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
# Both exes embed a Windows resource section (made with go-winres from
# packaging/windows/winres/winres.json.in): the Nudge icon (the same 16..256
# sizes as nudge.ico) as icon group resource ID 1, which Explorer and the
# taskbar show and the tray loads with LoadImageW(MAKEINTRESOURCE(1)); a
# version resource (ProductName "Nudge", FileDescription, CompanyName and
# LegalCopyright from LICENSE, OriginalFilename, FileVersion/ProductVersion =
# the version); and an application manifest (asInvoker, per-monitor v2 DPI
# aware, Common Controls v6). The .syso is generated and linked inside a temp
# copy of the Go sources, so no .syso ever appears in cmd/server.
#
# Needs: go (1.24), rsvg-convert, magick (ImageMagick 7), zip, sha256sum.
# go-winres is a pinned build tool run through `go run` (network or a warm
# module cache on the first run); it is not a dependency in go.mod.
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

# Fixed version fields need four numbers: 0.1.0 -> 0.1.0.0, 1.2.3-rc1 ->
# 1.2.3.0, a version that does not start with a number -> 0.0.0.0.
version4() {
  local num parts
  num="$(printf '%s' "$1" | grep -oE '^[0-9]+(\.[0-9]+){0,3}' || true)"
  IFS=. read -r -a parts <<< "${num:-0}"
  while ((${#parts[@]} < 4)); do parts+=(0); done
  for p in "${parts[@]}"; do
    if ((10#$p > 65535)); then
      echo "build-windows: version part $p does not fit a Windows version" >&2
      exit 2
    fi
  done
  printf '%d.%d.%d.%d\n' "$((10#${parts[0]}))" "$((10#${parts[1]}))" "$((10#${parts[2]}))" "$((10#${parts[3]}))"
}
version4="$(version4 "$version")"

# Pinned build tool (go run, not a go.mod dependency).
go_winres="github.com/tc-hib/go-winres@v0.3.3"

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
winres_tpl="$src_dir/winres/winres.json.in"
[[ -f "$winres_tpl" ]] || { echo "build-windows: missing $winres_tpl" >&2; exit 1; }

# CompanyName and LegalCopyright come from the LICENSE copyright line
# ("Copyright (c) 2026 Scag95" -> company "Scag95").
copyright="$(grep -m1 -E '^Copyright ' LICENSE 2>/dev/null || true)"
company="$(printf '%s' "$copyright" | sed -E 's/^Copyright[[:space:]]+(\([cC]\)[[:space:]]+)?[0-9][0-9, -]*[[:space:]]+//')"
if [[ -z "$copyright" || -z "$company" || "$copyright$company" == *[\"\\]* ]]; then
  echo "build-windows: cannot read a plain copyright line from LICENSE" >&2
  exit 1
fi

# Everything is staged in a temp dir that is always removed.
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
pkg="$stage/pkg"
mkdir -p "$pkg" "$stage/icon" "$stage/winres"

echo "==> nudge.ico"
sizes=(16 24 32 48 64 128 256)
pngs=()
for s in "${sizes[@]}"; do
  rsvg-convert -w "$s" -h "$s" -o "$stage/icon/$s.png" "$svg"
  pngs+=("$stage/icon/$s.png")
done
magick "${pngs[@]}" "$pkg/nudge.ico"
# go-winres resolves file names relative to the json file.
cp "$pkg/nudge.ico" "$stage/winres/nudge.ico"

# The Go sources are built from a temp copy so the .syso never lands in the
# repository (go build only links .syso files found in the package dir).
src="$stage/src"
mkdir -p "$src"
cp -R go.mod go.sum cmd internal "$src/"
find "$src" -name '*.syso' -type f -delete

# Escape a value for the replacement side of `sed s|...|...|`.
sed_repl() { printf '%s' "$1" | sed -e 's/[\\|&]/\\&/g'; }

# build_exe <exe name> <description> <extra ldflags>
build_exe() {
  local exe="$1" desc="$2" ldflags="$3" json="$stage/winres/${1%.exe}.json"
  sed -e "s|@DESCRIPTION@|$(sed_repl "$desc")|g" \
      -e "s|@VERSION4@|$version4|g" \
      -e "s|@VERSION@|$version|g" \
      -e "s|@COMPANY@|$(sed_repl "$company")|" \
      -e "s|@COPYRIGHT@|$(sed_repl "$copyright")|" \
      -e "s|@INTERNAL@|${exe%.exe}|" \
      -e "s|@FILENAME@|$exe|" \
      "$winres_tpl" > "$json"
  (cd "$stage/winres" && go run "$go_winres" make --in "$json" --arch amd64 --out "$src/cmd/server/rsrc")
  (
    cd "$src"
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
      go build -trimpath -buildvcs=false -ldflags "-s -w $ldflags" -o "$pkg/$exe" ./cmd/server
  )
  rm -f "$src/cmd/server/rsrc_windows_amd64.syso"
}

echo "==> go build (windows/amd64, version $version, resources $version4)"
desc="Nudge: portapapeles y control remoto desde el celu"
build_exe nudge.exe "$desc" ""
build_exe nudge-background.exe "$desc (segundo plano)" "-H=windowsgui"

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
