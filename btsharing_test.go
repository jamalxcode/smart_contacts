package main

import (
	"errors"
	"runtime"
	"strings"
	"testing"
)

func TestBluetoothSharingState(t *testing.T) {
	saved := readDefaults
	defer func() { readDefaults = saved }()
	missing := errors.New("exit status 1")
	cases := []struct {
		power, sharing string
		perr, serr     error
		want           string
		ok, known      bool
	}{
		{"1\n", "1\n", nil, nil, "Bluetooth Sharing is on", true, true},
		{"1\n", "0\n", nil, nil, "Bluetooth Sharing is off", false, true},
		// never switched on: macOS says the key doesn't exist
		{"1\n", "The domain/default pair of (com.apple.Bluetooth, PrefKeyServicesEnabled) does not exist\n", nil, missing, "Bluetooth Sharing is off", false, true},
		{"0\n", "1\n", nil, nil, "Bluetooth is off on this Mac", false, true},
		// power can't be read: go by sharing alone
		{"", "1\n", missing, nil, "Bluetooth Sharing is on", true, true},
		// nothing readable: say nothing
		{"", "garbage", missing, missing, "", false, false},
	}
	for i, c := range cases {
		readDefaults = func(args ...string) (string, error) {
			if strings.Contains(strings.Join(args, " "), "ControllerPowerState") {
				return c.power, c.perr
			}
			return c.sharing, c.serr
		}
		text, ok, known := bluetoothReady()
		if !strings.HasPrefix(text, c.want) || ok != c.ok || known != c.known {
			t.Errorf("case %d: %q ok=%v known=%v", i, text, ok, known)
		}
	}
	readDefaults = saved
	if _, _, known := bluetoothReady(); known && runtime.GOOS != "darwin" {
		t.Error("off a Mac there is nothing to read")
	}
}
