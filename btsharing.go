package main

import (
	"os/exec"
	"runtime"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Whether this Mac will take a file sent over Bluetooth (issue #6). Phones only say "refused" when
// Bluetooth Sharing is off, so the Bluetooth guide and the waiting window show the real state.
//
// Bluetooth Sharing is a per-user setting: `defaults -currentHost read com.apple.Bluetooth
// PrefKeyServicesEnabled` prints 1 when it's on and 0, or nothing (the key doesn't exist), when
// it's off — the same check the macOS 15 security guides use. Bluetooth's own power is in
// /Library/Preferences/com.apple.Bluetooth ControllerPowerState (1 = on). "Accept and Save" can't
// be read this way, so the guide still asks for it.

type btState int

const (
	btUnknown btState = iota // not a Mac, or macOS wouldn't say
	btOff
	btOn
)

// readDefaults runs /usr/bin/defaults (swapped in tests).
var readDefaults = func(args ...string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", errNotMac
	}
	out, err := exec.Command("/usr/bin/defaults", args...).CombinedOutput()
	return string(out), err
}

type notMac struct{}

func (notMac) Error() string { return "not a Mac" }

var errNotMac error = notMac{}

// parseBool reads a defaults value: 1/true on, 0/false off, a missing key off when missingOff.
func parseBool(out string, err error, missingOff bool) btState {
	v := strings.ToLower(strings.TrimSpace(out))
	if err != nil {
		if missingOff && strings.Contains(v, "does not exist") {
			return btOff
		}
		return btUnknown
	}
	switch v {
	case "1", "true", "yes":
		return btOn
	case "0", "false", "no":
		return btOff
	}
	return btUnknown
}

// bluetoothSharing reports whether Bluetooth Sharing is switched on for this user.
func bluetoothSharing() btState {
	out, err := readDefaults("-currentHost", "read", "com.apple.Bluetooth", "PrefKeyServicesEnabled")
	return parseBool(out, err, true) // never switched on: the key isn't there
}

// bluetoothPower reports whether Bluetooth itself is on.
func bluetoothPower() btState {
	out, err := readDefaults("read", "/Library/Preferences/com.apple.Bluetooth", "ControllerPowerState")
	return parseBool(out, err, false)
}

// bluetoothReady says, in a line, whether a phone can send a file here now, and whether all's well.
func bluetoothReady() (text string, ok bool, known bool) {
	if bluetoothPower() == btOff {
		return "Bluetooth is off on this Mac — turn it on in Control Centre or System Settings → Bluetooth.", false, true
	}
	switch bluetoothSharing() {
	case btOn:
		return "Bluetooth Sharing is on. Check it's set to Accept and Save (ⓘ next to it).", true, true
	case btOff:
		return "Bluetooth Sharing is off — the phone will say the Mac refused the file. Press Open Sharing Settings and turn it on.", false, true
	}
	return "", false, false
}

// btStatus is the line shown above the Bluetooth steps: a tick or a warning and what to do.
// Hidden where macOS won't say (or on another system).
type btStatus struct {
	obj  *fyne.Container
	icon *widget.Icon
	text *widget.Label
	last string
}

func newBTStatus() *btStatus {
	b := &btStatus{icon: widget.NewIcon(theme.ConfirmIcon()), text: widget.NewLabel("")}
	b.text.Wrapping = fyne.TextWrapWord
	b.text.TextStyle.Bold = true
	b.obj = container.NewBorder(nil, nil, container.NewVBox(b.icon), nil, b.text)
	b.obj.Hide()
	b.apply(bluetoothReady()) // built on the UI thread: show the state at once
	return b
}

// check reads the setting again from any goroutine; the screen is updated on Fyne's.
func (b *btStatus) check() {
	text, ok, known := bluetoothReady()
	fyne.Do(func() { b.apply(text, ok, known) })
}

func (b *btStatus) apply(text string, ok, known bool) {
	if !known {
		b.obj.Hide()
		return
	}
	if text == b.last && b.obj.Visible() {
		return
	}
	b.last = text
	b.text.SetText(text)
	if ok {
		b.icon.SetResource(theme.ConfirmIcon())
		b.text.Importance = widget.SuccessImportance
	} else {
		b.icon.SetResource(theme.WarningIcon())
		b.text.Importance = widget.DangerImportance
	}
	b.text.Refresh()
	b.obj.Show()
}
