#!/bin/bash
# Opens the real app window on a virtual screen and walks it by keyboard (issue #3).
#
# GitHub's Mac runners have no OpenGL, so the Mac window can't open in CI. Linux can draw with
# Mesa on Xvfb, which exercises the real GLFW window, the drawn Filofax, the menus' shortcuts and
# the windows they open — the parts the Fyne test driver doesn't reach. After each step the app
# must still be running and the screen must not be blank; every step's screenshot is kept.
#
#   .github/scripts/window-test.sh path/to/binary out-dir
set -euo pipefail
BIN="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"
OUT="${2:-window-test}"
mkdir -p "$OUT"
OUT="$(cd "$OUT" && pwd)"

export SMART_CONTACTS_DIR="$OUT/data" LIBGL_ALWAYS_SOFTWARE=1
rm -rf "$SMART_CONTACTS_DIR"
mkdir -p "$SMART_CONTACTS_DIR"
NEXT=$(( (RANDOM % 4) + 3 ))
ANNIV="2012-$(date -d "+$NEXT day" +%m-%d)"
# invented people only — the repo never holds real contacts
cat > "$OUT/people.vcf" <<EOF
BEGIN:VCARD
VERSION:3.0
N:Al-Kandari;Yousef;Ahmed;Dr.;PhD
X-PHONETIC-FIRST-NAME:Yoo-sef
ORG:Dar Al Shifa Hospital;
TITLE:Dentist
TEL;type=CELL:+965 9999 1111
EMAIL;type=WORK:yousef@example.com
BDAY:1984-03-14
item1.X-ABDATE:$ANNIV
item1.X-ABLabel:_\$!<Anniversary>!\$_
END:VCARD
BEGIN:VCARD
VERSION:3.0
N:Al-Mutairi;Khalid;;;
ORG:Boubyan Bank;
TEL;type=CELL:+965 6000 1234
END:VCARD
BEGIN:VCARD
VERSION:3.0
N:Haddad;Layla;;;
X-ALT-NAME:ليلى حداد
ORG:Gulf Bank;
TEL;type=CELL:+44 20 7946 0000
END:VCARD
EOF
"$BIN" --import "$OUT/people.vcf"

export DISPLAY=:97
Xvfb :97 -screen 0 1400x900x24 >"$OUT/xvfb.log" 2>&1 &
XVFB=$!
trap 'kill $APP $XVFB 2>/dev/null || true' EXIT
sleep 2
"$BIN" >"$OUT/app.log" 2>&1 &
APP=$!

fail() { echo "FAIL: $*"; echo "----- app output -----"; cat "$OUT/app.log"; exit 1; }
step=0
shot() { # shot <name>: the app is alive and the screen holds a picture, not one flat colour
  step=$((step + 1))
  sleep "${2:-2}"
  kill -0 $APP 2>/dev/null || fail "the app stopped before '$1'"
  local f="$OUT/$(printf '%02d' $step)-$1.png"
  import -window root "$f"
  local sd
  sd=$(convert "$f" -colorspace Gray -format "%[fx:standard_deviation]" info:)
  awk -v s="$sd" 'BEGIN { exit !(s > 0.03) }' || fail "'$1' looks blank (spread $sd)"
  echo "ok  $1  (spread $sd)"
}
key() { xdotool key --clearmodifiers "$@"; }
differs() { # differs <a> <b>: a window really opened or closed between two steps
  local n
  n=$(compare -metric AE -fuzz 2% "$OUT/$1" "$OUT/$2" null: 2>&1 || true)
  n=${n%% *}
  [ "${n%.*}" -gt 5000 ] 2>/dev/null || fail "'$2' looks the same as '$1' ($n pixels differ)"
  echo "ok  $1 → $2 changed ($n pixels)"
}

for i in $(seq 1 30); do
  WIN=$(xdotool search --name "Smart Contacts" 2>/dev/null | head -1 || true)
  [ -n "$WIN" ] && break
  sleep 1
done
[ -n "${WIN:-}" ] || fail "no window appeared"
xdotool windowactivate --sync "$WIN" 2>/dev/null || xdotool windowfocus "$WIN" || true

shot closed-cover 3
key space;          shot open-book 3         # any key opens it
key ctrl+1;         shot all-contacts
key Down;           shot first-card
key ctrl+0;         shot today               # the anniversary is this week
key ctrl+o;         shot import-wizard 3
key Escape;         shot wizard-closed;      differs 06-import-wizard.png 07-wizard-closed.png
key ctrl+1
key ctrl+f; xdotool type --delay 60 "Khlaid"
                    shot typo-search         # "no exact match, 1 close"
key Escape;         shot search-cleared;     differs 08-typo-search.png 09-search-cleared.png
key ctrl+shift+o;   shot quick-find
key Escape;         shot quick-find-closed;  differs 10-quick-find.png 11-quick-find-closed.png
key ctrl+comma;     shot settings            # a window whose first field takes the keyboard
key Escape;         shot settings-closed;    differs 12-settings.png 13-settings-closed.png
key ctrl+o;         shot wizard-again 3      # opens again after closing (it's built once)
key Escape;         shot wizard-again-closed; differs 14-wizard-again.png 15-wizard-again-closed.png

kill -0 $APP 2>/dev/null || fail "the app stopped at the end"
echo "All $step steps passed; screenshots in $OUT"
