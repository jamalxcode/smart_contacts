package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Updates come from the "latest" GitHub release, the same build the install command uses.
// Nothing is checked automatically: the app only goes online when you choose Check for Updates,
// and the request carries no contact data.
const (
	releaseAPI = "https://api.github.com/repos/jamalxcode/smart_contacts/releases/tags/latest"
	releaseZip = "https://github.com/jamalxcode/smart_contacts/releases/download/latest/Smart-Contacts-mac.zip"
)

// Release is what the update check finds.
type Release struct {
	Version string // "1.0.19"
	Build   int    // 19
	Notes   string // what changed
	When    time.Time
}

var versionRE = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// FetchLatest asks GitHub for the newest build.
func FetchLatest() (*Release, error) {
	req, _ := http.NewRequest("GET", releaseAPI, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "SmartContacts-updater")
	cl := &http.Client{Timeout: 20 * time.Second}
	resp, err := cl.Do(req)
	if err != nil {
		return nil, fmt.Errorf("couldn't reach GitHub — check the internet connection (%v)", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub answered %s; try again in a few minutes", resp.Status)
	}
	var r struct {
		Name      string    `json:"name"`
		Body      string    `json:"body"`
		Published time.Time `json:"published_at"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r); err != nil {
		return nil, fmt.Errorf("couldn't read GitHub's answer: %v", err)
	}
	return parseRelease(r.Name, r.Body, r.Published)
}

func parseRelease(name, body string, when time.Time) (*Release, error) {
	m := versionRE.FindStringSubmatch(name)
	if m == nil {
		return nil, errors.New("the latest release has no version number")
	}
	build, _ := strconv.Atoi(m[3])
	return &Release{Version: m[0], Build: build, Notes: strings.TrimSpace(body), When: when}, nil
}

// NewerThan reports whether the release is newer than the running version ("1.0.18").
func (r *Release) NewerThan(current string) bool {
	m := versionRE.FindStringSubmatch(current)
	if m == nil {
		return true
	}
	for i := 1; i <= 3; i++ {
		a, _ := strconv.Atoi(m[i])
		b, _ := strconv.Atoi(versionRE.FindStringSubmatch(r.Version)[i])
		if a != b {
			return b > a
		}
	}
	return false
}

// appBundle is the "Smart Contacts.app" folder the running program is in ("" for a dev build).
func appBundle() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	// …/Smart Contacts.app/Contents/MacOS/smartcontacts
	b := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	if !strings.HasSuffix(b, ".app") || strings.Contains(b, "/AppTranslocation/") {
		return ""
	}
	return b
}

// DownloadUpdate fetches and unpacks the new app next to a temporary folder, reporting progress
// (0–1). It returns the unpacked .app; nothing is replaced yet.
func DownloadUpdate(progress func(float64)) (string, error) {
	tmp, err := os.MkdirTemp("", "smart-contacts-update-")
	if err != nil {
		return "", err
	}
	zipPath := filepath.Join(tmp, "update.zip")
	cl := &http.Client{Timeout: 10 * time.Minute}
	req, _ := http.NewRequest("GET", releaseZip, nil)
	req.Header.Set("User-Agent", "SmartContacts-updater")
	resp, err := cl.Do(req)
	if err != nil {
		return "", fmt.Errorf("download failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download failed: %s", resp.Status)
	}
	f, err := os.Create(zipPath)
	if err != nil {
		return "", err
	}
	var done int64
	buf := make([]byte, 256<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				f.Close()
				return "", err
			}
			done += int64(n)
			if resp.ContentLength > 0 && progress != nil {
				progress(float64(done) / float64(resp.ContentLength))
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			return "", fmt.Errorf("download interrupted: %v", rerr)
		}
	}
	f.Close()
	if zr, err := zip.OpenReader(zipPath); err != nil {
		return "", errors.New("the download is damaged; try again")
	} else {
		zr.Close()
	}
	out := filepath.Join(tmp, "app")
	// ditto keeps the app's permissions and signature intact
	if b, err := exec.Command("ditto", "-x", "-k", zipPath, out).CombinedOutput(); err != nil {
		return "", fmt.Errorf("couldn't unpack the update: %s", strings.TrimSpace(string(b)))
	}
	app := filepath.Join(out, "Smart Contacts.app")
	if _, err := os.Stat(filepath.Join(app, "Contents", "Info.plist")); err != nil {
		return "", errors.New("the download doesn't contain the app; try again")
	}
	return app, nil
}

// InstallAndRelaunch starts a small helper that waits for this app to quit, swaps in the new
// one (keeping the old one until the new one is in place) and opens it. The caller then quits.
func InstallAndRelaunch(newApp, bundle string) error {
	if runtime.GOOS != "darwin" {
		return errors.New("updating only works on a Mac")
	}
	script := `pid="$1"; new="$2"; dest="$3"
while kill -0 "$pid" 2>/dev/null; do sleep 0.3; done
old="$dest.old-$$"
mv "$dest" "$old" || { open "$dest"; exit 1; }
if mv "$new" "$dest"; then
  rm -rf "$old"
  xattr -dr com.apple.quarantine "$dest" 2>/dev/null
else
  mv "$old" "$dest"
fi
rm -rf "$(dirname "$(dirname "$new")")"
open "$dest"
`
	cmd := exec.Command("/bin/bash", "-c", script, "updater", strconv.Itoa(os.Getpid()), newApp, bundle)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // keeps running after the app quits
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	return cmd.Start()
}

// canWrite reports whether the folder holding the app can be changed by this user.
func canWrite(dir string) bool {
	f, err := os.CreateTemp(dir, ".sc-write-test-")
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(f.Name())
	return true
}
