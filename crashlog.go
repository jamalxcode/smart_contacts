package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
)

// keepCrashLog writes what went wrong if the app ever quits unexpectedly, so the cause can be
// found and fixed. It goes in ~/Library/Logs/Smart Contacts (away from the contacts themselves):
// crash.log holds a crash from this run (the one before is kept as crash-previous.log), app.log the app's warnings. Neither contains any contact.
func keepCrashLog() {
	home, err := os.UserHomeDir()
	if err != nil || os.Getenv("SMART_CONTACTS_DIR") != "" && os.Getenv("SC_CRASHLOG") == "" {
		return
	}
	dir := filepath.Join(home, "Library", "Logs", "Smart Contacts")
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	crash := filepath.Join(dir, "crash.log")
	if st, err := os.Stat(crash); err == nil && st.Size() > 0 { // keep the last crash's report
		_ = os.Rename(crash, filepath.Join(dir, "crash-previous.log"))
	}
	if f, err := os.OpenFile(crash, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600); err == nil {
		_ = debug.SetCrashOutput(f, debug.CrashOptions{})
		f.Close() // SetCrashOutput keeps its own copy
	}
	if f, err := os.OpenFile(filepath.Join(dir, "app.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600); err == nil {
		log.SetOutput(f)
	}
}
