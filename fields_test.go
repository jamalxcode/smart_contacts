package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every text field is a cardEntry (newCardEntry, newMultiEntry, newPasswordEntry): Fyne's own Entry
// ignores Esc, and a focused field gets the key before the window, so a window with a plain field
// in it couldn't be closed with Esc.
func TestNoPlainTextFields(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, _ := os.ReadFile(f)
		for i, line := range strings.Split(string(b), "\n") {
			for _, bad := range []string{"widget.NewEntry(", "widget.NewMultiLineEntry(", "widget.NewPasswordEntry("} {
				if strings.Contains(line, bad) && !strings.HasPrefix(strings.TrimSpace(line), "//") {
					t.Errorf("%s:%d uses %s — use newCardEntry / newMultiEntry / newPasswordEntry so Esc closes its window", f, i+1, bad)
				}
			}
		}
	}
}
