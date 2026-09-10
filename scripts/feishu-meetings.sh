#!/bin/sh
set -eu

skill_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

case "$(uname -s)" in
  Linux) platform=linux ;;
  Darwin) platform=darwin ;;
  *) echo "Unsupported operating system: $(uname -s)" >&2; exit 2 ;;
esac

case "$(uname -m)" in
  x86_64|amd64) architecture=amd64 ;;
  arm64|aarch64) architecture=arm64 ;;
  *) echo "Unsupported architecture: $(uname -m)" >&2; exit 2 ;;
esac

binary="$skill_root/bin/$platform-$architecture/feishu-meetings"
if [ ! -f "$binary" ]; then
  echo "The standalone Feishu helper is missing: $binary" >&2
  exit 3
fi
if [ ! -x "$binary" ]; then
  chmod 700 "$binary"
fi

exec "$binary" "$@"

