#!/bin/bash
# Create the two native Linux images used by the macOS launcher.
set -euo pipefail
cd "$(dirname "$0")/.."
res="${1:?Supply a bundle directory}"
mkdir -p "$res"
temp="$(mktemp -d)"
trap 'rm -rf "$temp"' EXIT
docker build --iidfile "$temp/image-id" -t healthctl:desktop-build .
appimage="healthctl-local/app:$(cut -c8-23 "$temp/image-id")"
dbref="$(awk '/image: postgres:/{print $2;exit}' compose.yaml)"
docker image inspect "$dbref" >/dev/null 2>&1 || docker pull "$dbref"
dbimage="healthctl-local/db:${dbref#*@sha256:}"; dbimage="${dbimage:0:35}"
docker tag healthctl:desktop-build "$appimage"; docker tag "$dbref" "$dbimage"
printf '%s %s\n' "$appimage" "$dbimage" > "$res/images.txt"
docker save "$appimage" "$dbimage" | gzip -1 > "$res/images.tar.gz"
