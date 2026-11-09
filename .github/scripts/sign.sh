#!/bin/bash
# Signs "Smart Contacts.app" (issue #5).
#
# With the Apple secrets set (see README → Signing and notarisation) it signs with the Developer ID
# certificate, hardened runtime and a secure timestamp, has Apple notarise it and staples the
# ticket, so a zip downloaded in a browser opens without warnings. Without them it signs ad hoc,
# exactly as before, and the curl install line stays the way in.
#
# Secrets (GitHub → Settings → Secrets and variables → Actions):
#   APPLE_CERT_P12          base64 of the "Developer ID Application" certificate exported as .p12
#   APPLE_CERT_PASSWORD     the .p12's password
#   APPLE_TEAM_ID           the 10-character Team ID
#   APPLE_API_KEY_P8        base64 of the App Store Connect API key (.p8) used by notarytool
#   APPLE_API_KEY_ID        that key's ID
#   APPLE_API_ISSUER        the key's Issuer ID
set -euo pipefail
APP="${1:?usage: sign.sh path/to/App.app}"
ENT="$(dirname "$0")/entitlements.plist"

if [ -z "${APPLE_CERT_P12:-}" ] || [ -z "${APPLE_API_KEY_P8:-}" ]; then
  echo "No Apple Developer ID secrets: signing ad hoc."
  codesign --force --deep --sign - "$APP"
  codesign --verify --verbose "$APP"
  echo "SIGNED=adhoc" >> "${GITHUB_ENV:-/dev/null}"
  exit 0
fi

WORK="$(mktemp -d)"
trap 'security delete-keychain "$WORK/sign.keychain-db" 2>/dev/null || true; rm -rf "$WORK"' EXIT

# a keychain of its own for this run, unlocked, holding only the certificate
KC="$WORK/sign.keychain-db"
KCPASS="$(uuidgen)"
security create-keychain -p "$KCPASS" "$KC"
security set-keychain-settings -lut 3600 "$KC"
security unlock-keychain -p "$KCPASS" "$KC"
echo "$APPLE_CERT_P12" | base64 --decode > "$WORK/cert.p12"
security import "$WORK/cert.p12" -k "$KC" -P "$APPLE_CERT_PASSWORD" -T /usr/bin/codesign
security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "$KCPASS" "$KC" >/dev/null
security list-keychains -d user -s "$KC" $(security list-keychains -d user | tr -d '"')
IDENTITY="$(security find-identity -v -p codesigning "$KC" | awk -F'"' '/Developer ID Application/ {print $2; exit}')"
if [ -z "$IDENTITY" ]; then
  echo "::error::No 'Developer ID Application' identity in APPLE_CERT_P12"
  exit 1
fi
echo "Signing as: $IDENTITY"

# inside out: the binary, then the bundle (no --deep: it's deprecated for signing)
for f in "$APP"/Contents/MacOS/*; do
  codesign --force --timestamp --options runtime --entitlements "$ENT" --keychain "$KC" --sign "$IDENTITY" "$f"
done
codesign --force --timestamp --options runtime --entitlements "$ENT" --keychain "$KC" --sign "$IDENTITY" "$APP"
codesign --verify --strict --verbose=2 "$APP"

# notarise: Apple checks the zip and returns a ticket, which is stapled to the app
echo "$APPLE_API_KEY_P8" | base64 --decode > "$WORK/key.p8"
ditto -c -k --keepParent "$APP" "$WORK/notarise.zip"
if ! xcrun notarytool submit "$WORK/notarise.zip" --key "$WORK/key.p8" --key-id "$APPLE_API_KEY_ID" \
     --issuer "$APPLE_API_ISSUER" --team-id "$APPLE_TEAM_ID" --wait --timeout 30m --output-format json > "$WORK/notary.json"; then
  cat "$WORK/notary.json" || true
  echo "::error::notarytool submit failed"
  exit 1
fi
cat "$WORK/notary.json"
STATUS="$(/usr/bin/python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["status"])' "$WORK/notary.json")"
if [ "$STATUS" != "Accepted" ]; then
  ID="$(/usr/bin/python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["id"])' "$WORK/notary.json")"
  xcrun notarytool log "$ID" --key "$WORK/key.p8" --key-id "$APPLE_API_KEY_ID" --issuer "$APPLE_API_ISSUER" --team-id "$APPLE_TEAM_ID" || true
  echo "::error::Notarisation was $STATUS"
  exit 1
fi
xcrun stapler staple "$APP"
xcrun stapler validate "$APP"
spctl --assess --type execute --verbose=2 "$APP"
echo "SIGNED=developer-id" >> "${GITHUB_ENV:-/dev/null}"
