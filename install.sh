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
url="https://github.com/$repo/releases/download/$tag/lintmax-go_${os}_${arch}.tar.gz"
tmp=$(mktemp -d)
echo "downloading lintmax-go $tag ($os/$arch)…"
curl -fsSL "$url" | tar -xz -C "$tmp"
install -m 0755 "$tmp/lintmax-go" "$bindir/lintmax-go"
rm -rf "$tmp"
echo "installed to $bindir/lintmax-go"
