package main

import (
	"archive/zip"
	"context"
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
// GitHub serves release files from a separate storage server that sometimes answers
// "504 Gateway Timeout" or drops the connection. So the download tries two routes (the
// release link, then GitHub's API asset link), retries with pauses, and resumes a partial
// download instead of starting over.
var (
	// pauses between attempts; each attempt switches link, so both get tried early
	retryWaits   = []time.Duration{0, 0, 3 * time.Second, 3 * time.Second, 8 * time.Second, 8 * time.Second}
	downloadURLs = func() []string { return []string{assetAPIURL(), releaseZip} } // the API link first: it has been the reliable one
	headerWait   = 20 * time.Second                                               // GitHub must start answering within this
	stallWait    = 30 * time.Second                                               // and must not go quiet for longer than this mid-download
)

// assetAPIURL is the API link of the release's zip ("" if GitHub can't be asked).
func assetAPIURL() string {
	req, _ := http.NewRequest("GET", releaseAPI, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "SmartContacts-updater")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var r struct {
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"assets"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r) != nil {
		return ""
	}
	for _, a := range r.Assets {
		if a.Name == filepath.Base(releaseZip) {
			return a.URL
		}
	}
	return ""
}

// errPermanent is an answer that retrying won't change (404, 403…).
type errPermanent struct{ error }

// fetchOnce downloads url into path, resuming from what's already there.
func fetchOnce(ctx context.Context, cl *http.Client, url, path string, progress func(float64)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stall := time.AfterFunc(headerWait+stallWait, cancel) // reset whenever bytes arrive
	have := int64(0)
	if st, err := os.Stat(path); err == nil {
		have = st.Size()
	}
	defer stall.Stop()
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("User-Agent", "SmartContacts-updater")
	if strings.Contains(url, "api.github.com") {
		req.Header.Set("Accept", "application/octet-stream")
	}
	if have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
	}
	resp, err := cl.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	flags := os.O_CREATE | os.O_WRONLY
	total := resp.ContentLength
	switch {
	case resp.StatusCode == http.StatusPartialContent && have > 0:
		flags |= os.O_APPEND
		if total > 0 {
			total += have
		}
	case resp.StatusCode == http.StatusOK:
		flags |= os.O_TRUNC
		have = 0
	case resp.StatusCode == http.StatusRequestedRangeNotSatisfiable && have > 0:
		return nil // already complete
	case resp.StatusCode >= 500 || resp.StatusCode == 429:
		return fmt.Errorf("GitHub answered %s", resp.Status)
	default:
		return errPermanent{fmt.Errorf("GitHub answered %s", resp.Status)}
	}
	f, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		return errPermanent{err}
	}
	defer f.Close()
	done := have
	buf := make([]byte, 256<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			stall.Reset(stallWait)
			if _, err := f.Write(buf[:n]); err != nil {
				return errPermanent{err}
			}
			done += int64(n)
			if total > 0 && progress != nil {
				progress(float64(done) / float64(total))
			}
		}
		if rerr == io.EOF {
			if total > 0 && done < total {
				return fmt.Errorf("the connection closed early")
			}
			return nil
		}
		if rerr != nil {
			return rerr
		}
	}
}

// downloadZip tries the links in turn, switching after every failure, keeping what has
// arrived so the next attempt resumes. status tells the screen what's happening.
func downloadZip(ctx context.Context, path string, progress func(float64), status func(string)) error {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.ResponseHeaderTimeout = headerWait
	cl := &http.Client{Transport: tr} // no overall limit: a slow but moving download is fine
	var urls []string
	for _, u := range downloadURLs() {
		if u != "" {
			urls = append(urls, u)
		}
	}
	if len(urls) == 0 {
		return errors.New("couldn't find the update on GitHub; try again in a few minutes")
	}
	say := func(s string) {
		if status != nil {
			status(s)
		}
	}
	var last error
	gone := map[string]bool{} // links that answered "not found" or similar
	for i, wait := range retryWaits {
		url := urls[i%len(urls)]
		if gone[url] {
			continue
		}
		if i > 0 {
			say(fmt.Sprintf("GitHub is slow to answer — trying again (%d of %d)…", i+1, len(retryWaits)))
		}
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
		err := fetchOnce(ctx, cl, url, path, progress)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil {
			if zr, zerr := zip.OpenReader(path); zerr == nil {
				zr.Close()
				return nil
			}
			os.Remove(path)
			last = errors.New("the download was damaged")
			continue
		}
		last = err
		var p errPermanent
		if errors.As(err, &p) {
			gone[url] = true
			os.Remove(path)
		}
	}
	return fmt.Errorf("the download didn't finish: GitHub's file server is busy (%v). Try Check for Updates again in a few minutes, "+
		"or update from Terminal:\n\ncurl -fsSL https://raw.githubusercontent.com/jamalxcode/smart_contacts/main/install.sh | bash", last)
}

func DownloadUpdate(ctx context.Context, progress func(float64), status func(string)) (string, error) {
	tmp, err := os.MkdirTemp("", "smart-contacts-update-")
	if err != nil {
		return "", err
	}
	zipPath := filepath.Join(tmp, "update.zip")
	if err := downloadZip(ctx, zipPath, progress, status); err != nil {
		os.RemoveAll(tmp)
		return "", err
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
