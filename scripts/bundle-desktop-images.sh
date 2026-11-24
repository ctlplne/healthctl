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
# The launcher refuses to start images whose IDs differ from these.
printf '%s %s %s %s\n' "$appimage" "$dbimage" "$(cat "$temp/image-id")" "$(docker image inspect --format '{{.Id}}' "$dbimage")" > "$res/images.txt"
# Capture notices from the exact app image and catalog the database archive.
res="$(cd "$res" && pwd)"
mkdir -p "$res/notices/application"
container=$(docker create "$appimage")
trap 'docker rm "$container" >/dev/null; rm -rf "$temp"' EXIT
docker cp "$container:/usr/share/licenses/clinical-history/." "$res/notices/application/"
docker cp "$container:/usr/share/healthctl/app.spdx.json" "$res/notices/app.spdx.json"
docker save -o "$temp/database.tar" "$dbimage"
chmod a+r "$temp/database.tar"
scanner=$(sed -n 's/^ARG SYFT_IMAGE=//p' Dockerfile)
docker run --rm --network none -e SYFT_CHECK_FOR_APP_UPDATE=false \
    -v "$temp/database.tar:/database.tar:ro" "$scanner" scan docker-archive:/database.tar \
    --source-name healthctl-postgres -o spdx-json > "$res/notices/db.spdx.json"
docker save "$appimage" "$dbimage" | gzip -1 > "$res/images.tar.gz"
