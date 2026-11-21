#!/bin/bash
# Run on the target Mac architecture. Set HEALTHCTL_SIGN_IDENTITY and
# HEALTHCTL_NOTARY_PROFILE for a Developer ID signed and notarized build.
set -euo pipefail
cd "$(dirname "$0")/.."
mode=${1:-bundled}
[[ "$mode" == bundled || "$mode" == --build-on-first-use ]] || { echo "Usage: $0 [--build-on-first-use]" >&2; exit 2; }
root="$PWD"
dest="$root/build/desktop"; mkdir -p "$dest"
temp="$(mktemp -d "$dest/build-XXXXXXXX")";trap 'rm -rf "$temp"' EXIT
app="$temp/disk/healthctl.app"
bin="$app/Contents/MacOS";res="$app/Contents/Resources"
if [[ -n "${HEALTHCTL_TESTED_DESKTOP_APP:-}" ]]; then
  codesign --verify --deep --strict "$HEALTHCTL_TESTED_DESKTOP_APP"
  ditto "$HEALTHCTL_TESTED_DESKTOP_APP" "$app"
else
  mkdir -p "$bin" "$res"
  cp desktop/Info.plist "$app/Contents/Info.plist"
  cp compose.yaml desktop/desktop.yaml desktop/manage.sh LICENSE THIRD_PARTY_NOTICES.md "$res/"
  swiftc -O -target "$(uname -m)-apple-macos13.0" -module-cache-path "$dest/swift-cache" desktop/Launcher.swift -o "$bin/healthctl"
  go build -trimpath -ldflags="-s -w" -o "$res/archive" ./cmd/healthctl-archive
  if [[ "$mode" == --build-on-first-use ]]; then
    revision=$(git rev-parse HEAD)
    # Only tracked build inputs from the release revision go into the download.
    git archive --format=tar.gz -o "$res/healthctl-source.tar.gz" "$revision" -- \
      Dockerfile .dockerignore LICENSE PRIVACY.md THIRD_PARTY_NOTICES.md go.mod go.sum \
      cmd internal evaluation web docs/release.md docs/licenses/whisper-model.txt \
      scripts/go-notices.sh scripts/sbom-assets.mjs
    dbimage=$(awk '/image: postgres:/{print $2;exit}' "$res/compose.yaml")
    [[ "$dbimage" =~ @sha256:[[:xdigit:]]{64}$ ]]
    printf 'healthctl-local/app:%s %s\n' "$revision" "$dbimage" > "$res/images.txt"
  elif [[ -n "${HEALTHCTL_DESKTOP_IMAGES:-}" ]]; then
    cp -R "$HEALTHCTL_DESKTOP_IMAGES/notices" "$res/"
    cp "$HEALTHCTL_DESKTOP_IMAGES/images.txt" "$HEALTHCTL_DESKTOP_IMAGES/images.tar.gz" "$res/"
  else
    ./scripts/bundle-desktop-images.sh "$res"
  fi
  sh scripts/go-notices.sh "$res/notices/archive-go" "$res/archive"
  python3 scripts/desktop-sbom.py "$app"
fi
identity="${HEALTHCTL_SIGN_IDENTITY:--}"
sign=(--force --sign "$identity")
if [[ "$identity" != - ]]; then sign+=(--timestamp --options runtime); fi
codesign "${sign[@]}" "$res/archive"
codesign "${sign[@]}" "$app"
codesign --verify --strict --deep "$app"
ln -s /Applications "$temp/disk/Applications"
package="$dest/healthctl-$(uname -m).dmg"
if [[ "$mode" == --build-on-first-use ]]; then package="$dest/healthctl-$(uname -m)-preview.dmg"; fi
# Select the filesystem explicitly on hosted macOS. hdiutil can transiently
# fail to attach its temporary disk on a busy runner.
for delay in 1 2 4 8; do
  if hdiutil create -ov -fs HFS+ -volname healthctl -srcfolder "$temp/disk" -format UDZO "$temp/package.dmg"; then break; fi
  if [[ "$delay" == 8 ]]; then exit 1; fi
  sleep "$delay"
done
codesign "${sign[@]}" "$temp/package.dmg"
# macOS can briefly hold a newly signed image while examining its signature.
for delay in 1 2 4 8; do
 if hdiutil verify "$temp/package.dmg" >/dev/null; then break; fi
 if [[ "$delay" == 8 ]]; then exit 1; fi
 sleep "$delay"
done
if [[ -n "${HEALTHCTL_NOTARY_PROFILE:-}" ]];then
 [[ "$identity" != - ]] || { echo "Developer ID signing is required for notarization." >&2;exit 1; }
 notary=(--keychain-profile "$HEALTHCTL_NOTARY_PROFILE" --wait)
 if [[ -n "${HEALTHCTL_NOTARY_KEYCHAIN:-}" ]]; then notary+=(--keychain "$HEALTHCTL_NOTARY_KEYCHAIN"); fi
 xcrun notarytool submit "$temp/package.dmg" "${notary[@]}"
 xcrun stapler staple "$temp/package.dmg";xcrun stapler validate "$temp/package.dmg"
fi
mv "$temp/package.dmg" "$package"
printf 'Created %s\n' "$package"
