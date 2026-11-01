#!/usr/bin/env bash
# Print the -ldflags that stamp the version and revision into naminator.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
echo "-X main.version=$(git -C "$root" describe --tags --abbrev=0 --always) -X main.revision=$(git -C "$root" rev-parse --verify --short HEAD)"
