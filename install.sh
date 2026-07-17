#!/bin/sh
set -e
# lintmax-go installer — downloads the latest release binary for your platform.
# Go users can instead: go install github.com/1qh/lintmax-go@latest
repo="1qh/lintmax-go"
bindir="${BINDIR:-/usr/local/bin}"
os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "unsupported arch: $arch" >&2; exit 1 ;;
esac
tag=$(curl -fsSL "https://api.github.com/repos/$repo/releases/latest" | grep -m1 '"tag_name"' | cut -d'"' -f4)
[ -n "$tag" ] || { echo "could not resolve latest release" >&2; exit 1; }
ver=${tag#v}
url="https://github.com/$repo/releases/download/$tag/lintmax-go_${ver}_${os}_${arch}.tar.gz"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
echo "downloading lintmax-go $tag ($os/$arch)…"
# Piping the download straight into tar hides which step failed: plain sh has no pipefail, so a
# refused download leaves tar unpacking nothing and the run dies at `install` complaining about a
# missing file — blaming the file the download was supposed to bring rather than the download.
curl -fsSL "$url" -o "$tmp/lintmax-go.tar.gz" || { echo "download failed: $url" >&2; exit 1; }
tar -xzf "$tmp/lintmax-go.tar.gz" -C "$tmp" || { echo "cannot unpack $url" >&2; exit 1; }
install -m 0755 "$tmp/lintmax-go" "$bindir/lintmax-go"
echo "installed to $bindir/lintmax-go"
