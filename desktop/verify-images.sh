#!/bin/bash
# Check that local images are the ones a release recorded:
#   verify-images.sh APP_IMAGE APP_ID DB_IMAGE DB_ID
# An empty expected ID (an older bundle) is not checked.
set -euo pipefail
while (($#)); do
 image="$1" expected="${2:-}"; shift 2 || shift $#
 [[ -z "$expected" ]] && continue
 actual="$(docker image inspect --format '{{.Id}}' "$image" 2>/dev/null || true)"
 if [[ "$actual" != "$expected" ]]; then
  echo "image $image is ${actual:-missing}, expected $expected" >&2
  exit 1
 fi
done
