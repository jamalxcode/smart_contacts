#!/bin/bash
# Installs (or updates) Smart Contacts from the latest GitHub build.
#   curl -fsSL https://raw.githubusercontent.com/jamalxcode/smart_contacts/main/install.sh | bash
# Downloading with curl (not a browser) means macOS doesn't quarantine the app,
# so it opens without the "unidentified developer" warning.
set -euo pipefail
URL="https://github.com/jamalxcode/smart_contacts/releases/download/latest/Smart-Contacts-mac.zip"
TMP="$(mktemp -d)"
API="https://api.github.com/repos/jamalxcode/smart_contacts/releases/tags/latest"
echo "Downloading Smart Contacts…"
# GitHub's file server sometimes answers 504; retry, resume, then try the API link.
get() { curl -fL --retry 3 --retry-delay 3 --retry-all-errors -C - --progress-bar "$@" -o "$TMP/sc.zip"; }
# GitHub's API download link has been the reliable one; the plain release link is the backup.
ASSET="$(curl -fsSL "$API" 2>/dev/null | grep -o '"url": *"https://api.github.com/repos/[^"]*/releases/assets/[0-9]*"' | head -1 | sed 's/.*"\(https[^"]*\)"/\1/' || true)"
if [ -z "$ASSET" ] || ! get -H "Accept: application/octet-stream" "$ASSET"; then
  echo "Trying GitHub's other download link…"
  rm -f "$TMP/sc.zip"
  if ! get "$URL"; then
    echo "The download didn't finish — GitHub's file server is busy. Try again in a few minutes." >&2
    exit 1
  fi
fi
if ! unzip -tq "$TMP/sc.zip" >/dev/null 2>&1; then
  echo "The download was damaged — please run the command again." >&2
  exit 1
fi
pkill -x "smartcontacts" 2>/dev/null || true
pkill -x "Smart Contacts" 2>/dev/null || true
rm -rf "/Applications/Smart Contacts.app"
ditto -x -k "$TMP/sc.zip" /Applications
xattr -dr com.apple.quarantine "/Applications/Smart Contacts.app" 2>/dev/null || true
touch "/Applications/Smart Contacts.app" # so Finder and the Dock show the new icon
/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister -f "/Applications/Smart Contacts.app" 2>/dev/null || true
rm -rf "$TMP"
echo "Installed in /Applications. Opening…"
open "/Applications/Smart Contacts.app"
