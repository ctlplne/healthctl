#!/bin/bash
# Sign the already qualified application using a temporary hosted macOS keychain.
set -euo pipefail
umask 077
image="${1:?Supply the qualified ad-hoc DMG}"
: "${APPLE_CERTIFICATE_P12:?}" "${APPLE_CERTIFICATE_PASSWORD:?}" "${APPLE_NOTARY_KEY_P8:?}" "${APPLE_NOTARY_KEY_ID:?}"
work="$(mktemp -d "${RUNNER_TEMP:?}/healthctl-sign-XXXXXXXX")"
mount="$work/mounted"
keychain="$work/signing.keychain-db"
mounted=false
cleanup() {
  if [[ "$mounted" == true ]]; then
    hdiutil detach "$mount" >/dev/null || true
  fi
  security delete-keychain "$keychain" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT
mkdir "$mount"
hdiutil attach -readonly -nobrowse -mountpoint "$mount" "$image"
mounted=true
printf '%s' "$APPLE_CERTIFICATE_P12" | base64 -D > "$work/cert.p12"
printf '%s' "$APPLE_NOTARY_KEY_P8" > "$work/auth.p8"
password="$(openssl rand -hex 32)"
security create-keychain -p "$password" "$keychain"
security set-keychain-settings -lut 7200 "$keychain"
security unlock-keychain -p "$password" "$keychain"
security import "$work/cert.p12" -k "$keychain" -P "$APPLE_CERTIFICATE_PASSWORD" -T /usr/bin/codesign
security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "$password" "$keychain" >/dev/null
security list-keychains -d user -s "$keychain"
identity="$(security find-identity -v -p codesigning "$keychain" | awk '/"Developer ID Application:/ {print $2}')"
[[ "$identity" =~ ^[[:xdigit:]]{40}$ ]] || { echo "Expected one valid Developer ID Application certificate." >&2; exit 1; }
notary=(--key "$work/auth.p8" --key-id "$APPLE_NOTARY_KEY_ID" --keychain "$keychain")
if [[ -n "${APPLE_NOTARY_ISSUER:-}" ]]; then notary+=(--issuer "$APPLE_NOTARY_ISSUER"); fi
xcrun notarytool store-credentials healthctl-release "${notary[@]}"
unset APPLE_CERTIFICATE_P12 APPLE_CERTIFICATE_PASSWORD APPLE_NOTARY_KEY_P8
export HEALTHCTL_TESTED_DESKTOP_APP="$mount/healthctl.app"
export HEALTHCTL_SIGN_IDENTITY="$identity" HEALTHCTL_NOTARY_PROFILE=healthctl-release HEALTHCTL_NOTARY_KEYCHAIN="$keychain"
./scripts/build-desktop.sh
xcrun stapler validate build/desktop/healthctl-arm64.dmg
spctl --assess --type open --context context:primary-signature build/desktop/healthctl-arm64.dmg
