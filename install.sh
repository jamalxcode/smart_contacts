#!/bin/bash
# Installs (or updates) Smart Contacts from the latest GitHub build.
#   curl -fsSL https://raw.githubusercontent.com/jamalxcode/smart_contacts/main/install.sh | bash
# Downloading with curl (not a browser) means macOS doesn't quarantine the app,
# so it opens without the "unidentified developer" warning.
set -euo pipefail
URL="https://github.com/jamalxcode/smart_contacts/releases/download/latest/Smart-Contacts-mac.zip"
TMP="$(mktemp -d)"
echo "Downloading Smart Contacts…"
curl -fL --progress-bar -o "$TMP/sc.zip" "$URL"
pkill -x "smartcontacts" 2>/dev/null || true
pkill -x "Smart Contacts" 2>/dev/null || true
rm -rf "/Applications/Smart Contacts.app"
ditto -x -k "$TMP/sc.zip" /Applications
xattr -dr com.apple.quarantine "/Applications/Smart Contacts.app" 2>/dev/null || true
rm -rf "$TMP"
echo "Installed in /Applications. Opening…"
open "/Applications/Smart Contacts.app"
