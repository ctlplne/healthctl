#!/bin/bash
# Run on the target Mac architecture. Set HEALTHCTL_SIGN_IDENTITY and
# HEALTHCTL_NOTARY_PROFILE for a Developer ID signed and notarized build.
set -euo pipefail
cd "$(dirname "$0")/.."
root="$PWD"
dest="$root/build/desktop"; mkdir -p "$dest"
temp="$(mktemp -d "$dest/build-XXXXXXXX")";trap 'rm -rf "$temp"' EXIT
app="$temp/disk/healthctl.app"
bin="$app/Contents/MacOS";res="$app/Contents/Resources"
mkdir -p "$bin" "$res"
cp desktop/Info.plist "$app/Contents/Info.plist"
cp compose.yaml desktop/desktop.yaml desktop/manage.sh LICENSE "$res/"
swiftc -O -module-cache-path "$dest/swift-cache" desktop/Launcher.swift -o "$bin/healthctl"
go build -trimpath -ldflags="-s -w" -o "$res/archive" ./cmd/healthctl-archive
docker build --iidfile "$temp/image-id" -t healthctl:desktop-build .
appimage="healthctl-local/app:$(cut -c8-23 "$temp/image-id")"
dbref="$(awk '/image: postgres:/{print $2;exit}' compose.yaml)"
docker image inspect "$dbref" >/dev/null 2>&1 || docker pull "$dbref"
dbimage="healthctl-local/db:${dbref#*@sha256:}"; dbimage="${dbimage:0:35}"
docker tag healthctl:desktop-build "$appimage"; docker tag "$dbref" "$dbimage"
printf '%s %s\n' "$appimage" "$dbimage" > "$res/images.txt"
docker save "$appimage" "$dbimage" | gzip -1 > "$res/images.tar.gz"
identity="${HEALTHCTL_SIGN_IDENTITY:--}"
sign=(--force --sign "$identity")
if [[ "$identity" != - ]]; then sign+=(--timestamp --options runtime); fi
codesign "${sign[@]}" "$res/archive"
codesign "${sign[@]}" "$app"
codesign --verify --strict --deep "$app"
ln -s /Applications "$temp/disk/Applications"
package="$dest/healthctl-$(uname -m).dmg"
hdiutil create -quiet -volname healthctl -srcfolder "$temp/disk" -format UDZO "$temp/package.dmg"
codesign "${sign[@]}" "$temp/package.dmg"
# macOS can briefly hold a newly signed image while examining its signature.
for delay in 1 2 4 8; do
 if hdiutil verify "$temp/package.dmg" >/dev/null; then break; fi
 if [[ "$delay" == 8 ]]; then exit 1; fi
 sleep "$delay"
done
if [[ -n "${HEALTHCTL_NOTARY_PROFILE:-}" ]];then
 [[ "$identity" != - ]] || { echo "Developer ID signing is required for notarization." >&2;exit 1; }
 xcrun notarytool submit "$temp/package.dmg" --keychain-profile "$HEALTHCTL_NOTARY_PROFILE" --wait
 xcrun stapler staple "$temp/package.dmg";xcrun stapler validate "$temp/package.dmg"
fi
mv "$temp/package.dmg" "$package"
printf 'Created %s\n' "$package"
