#!/bin/bash
# Build Smart Contacts on this Mac (double-click in Finder, or run ./build.command).
# Needs Go (brew install go) and the Xcode command-line tools (xcode-select --install).
# You don't normally need this: GitHub builds the app on every push (see the README).
set -euo pipefail
cd "$(dirname "$0")"
export PATH="$PATH:$(go env GOPATH)/bin"
go mod tidy
go test ./...
command -v fyne >/dev/null || go install fyne.io/tools/cmd/fyne@latest
fyne package --os darwin --release
codesign --force --deep --sign - "Smart Contacts.app"
pkill -x smartcontacts 2>/dev/null || true
rm -rf "/Applications/Smart Contacts.app"
mv "Smart Contacts.app" /Applications/
open "/Applications/Smart Contacts.app"
echo "Done — Smart Contacts is in /Applications."
