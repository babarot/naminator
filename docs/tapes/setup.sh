#!/usr/bin/env bash
# Setup for demo.tape: a memory card copied to ~/Pictures/import, with a
# camera's RAW and JPEG pairs (two shots in the same second among them), an
# iPhone folder with a sidecar file and a screenshot without EXIF data, and
# two photos already named. The photos are a few bytes each; mock-exiftool
# stands in for exiftool and gives their shooting times.
set -euo pipefail

cp /data/.naminator /usr/local/bin/naminator
cp /data/mock-exiftool /usr/local/bin/exiftool
chmod +x /usr/local/bin/naminator /usr/local/bin/exiftool

export MOCK_DATES=/tmp/mock-dates MOCK_DELAY=2
export LANG=C.UTF-8
export XDG_DATA_HOME=/tmp/xdg
: > "$MOCK_DATES"

jpeg() { printf '\xff\xd8\xff\xe0\x00\x10JFIF\x00\x01\x01\x00\x00\x01\x00\x01\x00\x00\xff\xd9' > "$1"; }
raw() { printf 'II*\x00\x08\x00\x00\x00\x00\x00' > "$1"; }
# photo <path> <epoch seconds> [model]: a photo taken then, with that camera
photo() {
  case "$1" in *.ARW) raw "$1" ;; *) jpeg "$1" ;; esac
  echo "$(basename "$1")|$(date -u -d "@$2" '+%Y:%m:%d %H:%M:%S')|${3:-}" >> "$MOCK_DATES"
}

cd ~
mkdir -p Pictures/import/DCIM/100MSDCF Pictures/import/iPhone Pictures/import/2026-09-28
cd Pictures/import

t=$(date -u -d '2026-10-03 14:22:05' +%s)
steps=(4 9 0 13 6 21 3 8)
for i in "${!steps[@]}"; do
  t=$((t + steps[i]))
  n=$((401 + i))
  photo "DCIM/100MSDCF/DSC00$n.ARW" "$t" ILCE-7CM2
  photo "DCIM/100MSDCF/DSC00$n.JPG" "$t" ILCE-7CM2
done

t=$(date -u -d '2026-10-04 09:10:00' +%s)
for n in $(seq 2031 2035); do
  t=$((t + 97 * (n % 5 + 1)))
  photo "iPhone/IMG_$n.JPG" "$t" "iPhone 15 Pro"
done
echo '<sidecar/>' > iPhone/IMG_2033.AAE
jpeg iPhone/screenshot.jpg

for d in '2026-09-28 10:12:44' '2026-09-28 10:15:02'; do
  t=$(date -u -d "$d" +%s)
  photo "2026-09-28/$(date -u -d "@$t" '+%Y-%m-%d_%H-%M-%S').jpg" "$t" X100VI
done

cd ~/Pictures
export PS1='\[\e[34m\]~/Pictures\[\e[0m\] $ '
