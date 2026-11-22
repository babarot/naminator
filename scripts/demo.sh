#!/usr/bin/env bash
# Record the demo GIFs with VHS in Docker and copy them to docs/.
#
# Usage: demo.sh [-e KEY=VAL ...] [NAME ...]
#   NAME  a tape to record, without .tape (demo, ...); all when none
#   -e    an environment variable for the recording containers
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
tapes_dir="$root/docs/tapes"

names=()
args=("$@")
while [[ $# -gt 0 ]]; do
  case "$1" in
    -e) shift 2 ;;
    *) names+=("$1"); shift ;;
  esac
done

if ! docker info > /dev/null 2>&1; then
  echo "Docker is not running; start Docker and try again." >&2
  exit 1
fi

# The binary for the architecture the VHS image runs on
arch="$(docker version --format '{{.Server.Arch}}')"
echo "Building naminator for linux/$arch..."
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -ldflags "$("$root/scripts/ldflags.sh")" -trimpath -o "$tapes_dir/.naminator" "$root"
trap 'rm -f "$tapes_dir/.naminator"' EXIT

"$tapes_dir/vhs.sh" ${args[@]+"${args[@]}"}

if [[ ${#names[@]} -eq 0 ]]; then
  for tape in "$tapes_dir"/*.tape; do
    names+=("$(basename "$tape" .tape)")
  done
fi

echo "Copying the GIFs to docs/..."
for name in "${names[@]}"; do
  cp "$tapes_dir/$name.gif" "$root/docs/"
done
