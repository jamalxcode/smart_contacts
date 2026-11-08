package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// macContactsScript reads every card from the Contacts app as vCard text. AppleScript gives up
// on a slow app after 2 minutes; big address books with photos need longer.
const macContactsScript = `with timeout of 900 seconds
	tell application "Contacts"
		set cards to vcard of every person
	end tell
end timeout
set AppleScript's text item delimiters to linefeed
return cards as text`

// ReadMacContacts asks the built-in Contacts app for every card as vCard text.
// The first time, macOS asks the user to allow Smart Contacts to control Contacts.
func ReadMacContacts() ([]Card, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("this only works on a Mac")
	}
	script := macContactsScript
	cmd := exec.Command("osascript", "-e", script)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if strings.Contains(msg, "-1712") {
			return nil, errors.New("the Contacts app took too long to answer; try again, or export a .vcf from Contacts (File → Export) and import that")
		}
		if strings.Contains(msg, "-1743") || strings.Contains(strings.ToLower(msg), "not authorized") {
			return nil, errors.New("macOS blocked access to Contacts. Open System Settings → Privacy & Security → Automation, " +
				"turn on Contacts under Smart Contacts, then try again")
		}
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("couldn't read Mac Contacts: %s", msg)
	}
	return ParseVCards(&out)
}

// OpenInMacContacts writes the cards to a temporary .vcf and opens it, so the Contacts app
// offers to add them (it shows its own duplicate check).
func OpenInMacContacts(s *Store, cs []*Contact) error {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("Smart Contacts %s.vcf", time.Now().Format("2006-01-02 1504")))
	var b bytes.Buffer
	for _, c := range cs {
		if err := WriteVCard(&b, c, s.Photo(c)); err != nil {
			return err
		}
	}
	if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
		return err
	}
	if runtime.GOOS != "darwin" {
		os.Remove(path)
		return errors.New("this only works on a Mac")
	}
	err := exec.Command("open", "-a", "Contacts", path).Run()
	go func() { // don't leave a copy of the cards lying in the temp folder
		time.Sleep(2 * time.Minute)
		os.Remove(path)
	}()
	return err
}

// RevealInFinder shows a folder or file in Finder.
func RevealInFinder(path string) {
	if runtime.GOOS == "darwin" {
		_ = exec.Command("open", "-R", path).Start()
	} else {
		_ = exec.Command("xdg-open", filepath.Dir(path)).Start()
	}
}
