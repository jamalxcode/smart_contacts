package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Store keeps every contact in one JSON file, photos as separate files beside it,
// a daily backup, and a trash so nothing is lost by one wrong click.
//
//	<dir>/contacts.json
//	<dir>/photos/<id>
//	<dir>/backups/contacts-YYYY-MM-DD.json   (last 30 kept)
type Store struct {
	mu       sync.Mutex
	dir      string
	Contacts []*Contact
	Trash    []TrashItem
	// ExtraGroups are groups the user created that have no members yet.
	ExtraGroups []string
	stepSerial  int // counts recorded undo steps (see StepSerial)
	// Links are the relationships the user recorded between two contacts.
	Links []Link

	// What was last written: each card's JSON, the order of cards, and the rest (trash, groups,
	// links). A save compares against it, so an undo step holds only the cards that changed.
	saved      bool
	lastJSON   map[string][]byte
	lastOrder  []string
	lastMeta   []byte
	lastHash   [32]byte
	undo, redo []undoStep
}

// undoStep is what one change replaced: the earlier JSON of each card it touched (nil for a card
// that didn't exist), the earlier order of cards, and the earlier trash/groups/links.
type undoStep struct {
	Label  string            `json:"label"`
	Time   time.Time         `json:"time"`
	Before map[string][]byte `json:"before"`
	Order  []string          `json:"order"`
	Meta   []byte            `json:"meta"`
}

const maxUndo = 100

func (u undoStep) size() int {
	n := len(u.Meta) + 40*len(u.Order)
	for _, b := range u.Before {
		n += len(b)
	}
	return n
}

// storeMeta is everything in the file except the cards.
type storeMeta struct {
	Trash  []TrashItem `json:"trash,omitempty"`
	Groups []string    `json:"groups,omitempty"`
	Links  []Link      `json:"links,omitempty"`
}

// TrashItem is a card moved to the Trash, kept until the Trash is emptied.
type TrashItem struct {
	Contact *Contact  `json:"contact"`
	Deleted time.Time `json:"deleted"`
}

type storeFile struct {
	Version  int         `json:"version"`
	Saved    time.Time   `json:"saved"`
	Contacts []*Contact  `json:"contacts"`
	Trash    []TrashItem `json:"trash,omitempty"`
	Groups   []string    `json:"groups,omitempty"`
	Links    []Link      `json:"links,omitempty"`
}

const (
	keepBackups = 30
	trashDays   = 30
)

// DataDir is ~/Library/Application Support/Smart Contacts on a Mac.
func DataDir() string {
	if d := os.Getenv("SMART_CONTACTS_DIR"); d != "" {
		return d
	}
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "Smart Contacts")
}

// OpenStore loads the store in dir, creating it if needed.
func OpenStore(dir string) (*Store, error) {
	s := &Store{dir: dir}
	if err := os.MkdirAll(filepath.Join(dir, "photos"), 0o700); err != nil {
		return nil, err
	}
	lockDown(dir)
	b, err := os.ReadFile(s.file())
	if errors.Is(err, os.ErrNotExist) {
		s.remember(nil, nil, nil) // so the first change can be undone
		s.saved = true
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var f storeFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("contacts.json is damaged (%v); the daily backups are in %s", err, filepath.Join(dir, "backups"))
	}
	s.Contacts, s.Trash, s.ExtraGroups, s.Links = f.Contacts, f.Trash, f.Groups, f.Links
	// trashed cards stay until the Trash is emptied (Card → Empty Trash)
	order, js, meta := s.encode()
	s.remember(order, js, meta)
	s.saved = true
	s.loadUndo(b)
	return s, nil
}

func (s *Store) file() string { return filepath.Join(s.dir, "contacts.json") }

// lockDown makes the data folder private to this Mac user: folders 0700, files 0600, and a
// .metadata_never_index marker so Spotlight doesn't copy contact details into its index.
func lockDown(dir string) {
	_ = os.WriteFile(filepath.Join(dir, ".metadata_never_index"), nil, 0o600)
	_ = filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if fi.IsDir() {
			_ = os.Chmod(p, 0o700)
		} else if fi.Mode().Perm()&0o077 != 0 {
			_ = os.Chmod(p, 0o600)
		}
		return nil
	})
}

// InCloudFolder reports whether a path is inside a folder that syncs off this Mac
// (iCloud Drive, Dropbox, Google Drive, OneDrive, Box…).
func InCloudFolder(path string) bool {
	p := strings.ToLower(filepath.ToSlash(path))
	for _, m := range []string{"/library/mobile documents/", "/library/cloudstorage/", "/dropbox", "/google drive", "/googledrive", "/onedrive", "/box/", "/icloud drive"} {
		if strings.Contains(p, m) {
			return true
		}
	}
	return false
}

// Dir is where the data lives.
func (s *Store) Dir() string { return s.dir }

// Save writes atomically (temp file + rename) and makes the day's backup first.
func (s *Store) Save() error { return s.save(true) }

func (s *Store) save(record bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backupLocked(time.Now())
	order, js, meta := s.encode()
	file := assemble(order, js, meta)
	if err := writeFileAtomic(s.file(), file, 0o600); err != nil {
		return err
	}
	s.lastHash = sha256.Sum256(file)
	if record && s.saved {
		step := s.diff(order, js, meta)
		if label := describeStep(step, js, meta); label != "" {
			step.Label, step.Time = label, time.Now()
			s.undo = append(s.undo, step)
			s.stepSerial++
			if len(s.undo) > maxUndo {
				s.undo = s.undo[len(s.undo)-maxUndo:]
			}
			s.redo = nil
		}
	}
	s.remember(order, js, meta)
	s.saved = true
	return nil
}

// encode turns every card into JSON (in parallel: big address books have thousands) and the
// rest of the file into one more piece.
func (s *Store) encode() ([]string, map[string][]byte, []byte) {
	order := make([]string, len(s.Contacts))
	out := make([][]byte, len(s.Contacts))
	workers := runtime.NumCPU()
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := w; i < len(s.Contacts); i += workers {
				out[i], _ = json.Marshal(s.Contacts[i])
			}
		}(w)
	}
	wg.Wait()
	js := make(map[string][]byte, len(s.Contacts))
	for i, c := range s.Contacts {
		order[i] = c.ID
		js[c.ID] = out[i]
	}
	meta, _ := json.Marshal(storeMeta{Trash: s.Trash, Groups: s.ExtraGroups, Links: s.Links})
	return order, js, meta
}

// assemble writes the file: {"version":1,"saved":…,"contacts":[…],"trash":…,"groups":…,"links":…}
func assemble(order []string, js map[string][]byte, meta []byte) []byte {
	var b bytes.Buffer
	saved, _ := json.Marshal(time.Now())
	b.WriteString(`{"version":1,"saved":`)
	b.Write(saved)
	b.WriteString(`,"contacts":[`)
	for i, id := range order {
		if i > 0 {
			b.WriteString(",\n")
		} else {
			b.WriteString("\n")
		}
		b.Write(js[id])
	}
	b.WriteString("\n]")
	if m := bytes.TrimSpace(meta); len(m) > 2 { // {"trash":…} → ,"trash":…
		b.WriteByte(',')
		b.Write(m[1 : len(m)-1])
	}
	b.WriteString("}\n")
	return b.Bytes()
}

func (s *Store) remember(order []string, js map[string][]byte, meta []byte) {
	if js == nil {
		js = map[string][]byte{}
	}
	if meta == nil {
		meta = []byte("{}")
	}
	s.lastOrder, s.lastJSON, s.lastMeta = order, js, meta
}

// diff is the undo step that takes the new state back to the last saved one.
func (s *Store) diff(order []string, js map[string][]byte, meta []byte) undoStep {
	st := undoStep{Before: map[string][]byte{}, Order: s.lastOrder}
	for id, b := range js {
		if old, ok := s.lastJSON[id]; !ok {
			st.Before[id] = nil
		} else if !bytes.Equal(old, b) {
			st.Before[id] = old
		}
	}
	for id, old := range s.lastJSON {
		if _, ok := js[id]; !ok {
			st.Before[id] = old
		}
	}
	if !bytes.Equal(meta, s.lastMeta) {
		st.Meta = s.lastMeta
	}
	return st
}

// apply puts the store into the state a step describes (relative to the last saved state) and
// returns the step that reverses it.
func (s *Store) apply(st undoStep) error {
	cur := map[string]*Contact{}
	for _, c := range s.Contacts {
		cur[c.ID] = c
	}
	var cs []*Contact
	for _, id := range st.Order {
		if b, changed := st.Before[id]; changed {
			if b == nil {
				continue
			}
			var c Contact
			if err := json.Unmarshal(b, &c); err != nil {
				return err
			}
			cs = append(cs, &c)
		} else if c := cur[id]; c != nil {
			cs = append(cs, c)
		}
	}
	s.Contacts = cs
	if st.Meta != nil {
		var m storeMeta
		if err := json.Unmarshal(st.Meta, &m); err != nil {
			return err
		}
		s.Trash, s.ExtraGroups, s.Links = m.Trash, m.Groups, m.Links
	}
	return s.save(false)
}

// diffBack is the step that would undo applying st: the current JSON of every card st touches.
func (s *Store) diffBack(st undoStep) undoStep {
	r := undoStep{Label: st.Label, Time: time.Now(), Before: map[string][]byte{}, Order: s.lastOrder}
	for id := range st.Before {
		r.Before[id] = s.lastJSON[id] // nil when the card doesn't exist now
	}
	if st.Meta != nil {
		r.Meta = s.lastMeta
	}
	return r
}

// describeStep names a change from the cards it touched, ignoring changes to timestamps and
// "last contacted" alone ("" means nothing worth an undo step).
func describeStep(st undoStep, after map[string][]byte, afterMeta []byte) string {
	strip := func(b []byte) (string, string) {
		var c Contact
		if json.Unmarshal(b, &c) != nil {
			return string(b), ""
		}
		name := c.DisplayName()
		c.Updated, c.LastContacted, c.Created = time.Time{}, time.Time{}, time.Time{}
		j, _ := json.Marshal(&c)
		return string(j), name
	}
	var added, changed, removed []string
	for id, old := range st.Before {
		now, exists := after[id]
		switch {
		case old == nil && exists:
			_, n := strip(now)
			added = append(added, n)
		case old != nil && !exists:
			_, n := strip(old)
			removed = append(removed, n)
		case old != nil && exists:
			a, _ := strip(old)
			b, n := strip(now)
			if a != b {
				changed = append(changed, n)
			}
		}
	}
	sort.Strings(added)
	sort.Strings(changed)
	sort.Strings(removed)
	name := func(verb string, names []string) string {
		if len(names) == 1 {
			return verb + " " + names[0]
		}
		return fmt.Sprintf("%s %d people", verb, len(names))
	}
	var parts []string
	if len(added) > 0 {
		parts = append(parts, name("Added", added))
	}
	if len(changed) > 0 {
		parts = append(parts, name("Changed", changed))
	}
	if len(removed) > 0 {
		parts = append(parts, name("Deleted", removed))
	}
	if len(parts) == 0 && st.Meta != nil && !bytes.Equal(st.Meta, afterMeta) {
		var x, y storeMeta
		_ = json.Unmarshal(st.Meta, &x)
		_ = json.Unmarshal(afterMeta, &y)
		switch {
		case len(x.Links) != len(y.Links):
			parts = append(parts, "Changed connections")
		case strings.Join(x.Groups, "|") != strings.Join(y.Groups, "|"):
			parts = append(parts, "Changed groups")
		default:
			parts = append(parts, "Changed the Trash")
		}
	}
	return strings.Join(parts, ", ")
}

// CanUndo / CanRedo name the change that Undo or Redo would reverse ("" if none).
func (s *Store) CanUndo() string {
	if len(s.undo) == 0 {
		return ""
	}
	return s.undo[len(s.undo)-1].Label
}

func (s *Store) CanRedo() string {
	if len(s.redo) == 0 {
		return ""
	}
	return s.redo[len(s.redo)-1].Label
}

// Undo puts the address book back as it was before the last change.
func (s *Store) Undo() (string, error) {
	if len(s.undo) == 0 {
		return "", errors.New("nothing to undo")
	}
	step := s.undo[len(s.undo)-1]
	s.undo = s.undo[:len(s.undo)-1]
	s.redo = append(s.redo, s.diffBack(step))
	return step.Label, s.apply(step)
}

// Redo re-applies the last undone change.
func (s *Store) Redo() (string, error) {
	if len(s.redo) == 0 {
		return "", errors.New("nothing to redo")
	}
	step := s.redo[len(s.redo)-1]
	s.redo = s.redo[:len(s.redo)-1]
	s.undo = append(s.undo, s.diffBack(step))
	return step.Label, s.apply(step)
}

// History lists this session's changes, newest first.
func (s *Store) History() []undoStep {
	out := make([]undoStep, len(s.undo))
	for i := range s.undo {
		out[len(s.undo)-1-i] = s.undo[i]
	}
	return out
}

// UndoTo undoes every change back to and including history entry n (0 = the latest).
func (s *Store) UndoTo(n int) error {
	for i := 0; i <= n; i++ {
		if _, err := s.Undo(); err != nil {
			return err
		}
	}
	return nil
}

// RestoreFile replaces the address book with a backup file (the change itself can be undone).
func (s *Store) RestoreFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var f storeFile
	if err := json.Unmarshal(b, &f); err != nil {
		return fmt.Errorf("that backup is damaged: %v", err)
	}
	_, _ = s.SnapshotBackup("before-restore")
	s.Contacts, s.Trash, s.ExtraGroups, s.Links = f.Contacts, f.Trash, f.Groups, f.Links
	return s.Save()
}

// RestorePoints lists the backup files, newest first.
func (s *Store) RestorePoints() []string {
	files, _ := filepath.Glob(filepath.Join(s.dir, "backups", "*.json"))
	sort.Slice(files, func(i, j int) bool {
		a, _ := os.Stat(files[i])
		b, _ := os.Stat(files[j])
		return a != nil && b != nil && a.ModTime().After(b.ModTime())
	})
	return files
}

func writeFileAtomic(path string, b []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	tmp.Close()
	_ = os.Chmod(tmp.Name(), perm)
	return os.Rename(tmp.Name(), path)
}

// backupLocked copies today's starting state once per day and keeps the last 30.
func (s *Store) backupLocked(now time.Time) {
	src, err := os.ReadFile(s.file())
	if err != nil {
		return
	}
	bdir := filepath.Join(s.dir, "backups")
	_ = os.MkdirAll(bdir, 0o700)
	name := filepath.Join(bdir, "contacts-"+now.Format("2006-01-02")+".json")
	if _, err := os.Stat(name); err == nil {
		return
	}
	_ = os.WriteFile(name, src, 0o600)
	entries, _ := filepath.Glob(filepath.Join(bdir, "contacts-*.json"))
	sort.Strings(entries)
	for len(entries) > keepBackups {
		os.Remove(entries[0])
		entries = entries[1:]
	}
}

// SnapshotBackup writes the current contacts to backups/<tag>-<time>.json before a big change.
func (s *Store) SnapshotBackup(tag string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bdir := filepath.Join(s.dir, "backups")
	if err := os.MkdirAll(bdir, 0o700); err != nil {
		return "", err
	}
	f := storeFile{Version: 1, Saved: time.Now(), Contacts: s.Contacts, Trash: s.Trash, Groups: s.ExtraGroups, Links: s.Links}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return "", err
	}
	name := filepath.Join(bdir, tag+"-"+time.Now().Format("2006-01-02-150405")+".json")
	if err := writeFileAtomic(name, b, 0o600); err != nil {
		return "", err
	}
	// keep the 10 newest snapshots of each kind
	old, _ := filepath.Glob(filepath.Join(bdir, tag+"-*.json"))
	sort.Strings(old)
	for len(old) > 10 {
		os.Remove(old[0])
		old = old[1:]
	}
	return name, nil
}

// Get finds a contact by ID.
func (s *Store) Get(id string) *Contact {
	for _, c := range s.Contacts {
		if c.ID == id {
			return c
		}
	}
	return nil
}

// Put adds a new contact or replaces the one with the same ID.
func (s *Store) Put(c *Contact) {
	c.Tidy()
	c.Updated = time.Now()
	if c.Created.IsZero() || c.Created.After(c.Updated) {
		c.Created = c.Updated
	}
	for i, x := range s.Contacts {
		if x.ID == c.ID {
			s.Contacts[i] = c
			return
		}
	}
	s.Contacts = append(s.Contacts, c)
}

// Delete moves contacts to the trash.
func (s *Store) Delete(ids ...string) {
	kill := map[string]bool{}
	for _, id := range ids {
		kill[id] = true
	}
	keep := s.Contacts[:0]
	now := time.Now()
	for _, c := range s.Contacts {
		if kill[c.ID] {
			s.Trash = append(s.Trash, TrashItem{Contact: c, Deleted: now})
		} else {
			keep = append(keep, c)
		}
	}
	s.Contacts = keep
}

// Restore brings a contact back from the trash.
func (s *Store) Restore(id string) bool {
	for i, t := range s.Trash {
		if t.Contact.ID == id {
			s.Contacts = append(s.Contacts, t.Contact)
			s.Trash = append(s.Trash[:i], s.Trash[i+1:]...)
			return true
		}
	}
	return false
}

// EmptyTrash deletes trashed contacts and their photos for good.
func (s *Store) EmptyTrash() {
	defer s.dropDeadLinks()
	for _, t := range s.Trash {
		os.Remove(s.PhotoPath(t.Contact.ID))
		os.Remove(s.PhotoPath(t.Contact.ID) + ".removed")
		s.removeCardFiles(t.Contact)
	}
	s.Trash = nil
}

func (s *Store) purgeTrash(now time.Time) {
	defer s.dropDeadLinks()
	keep := s.Trash[:0]
	for _, t := range s.Trash {
		if now.Sub(t.Deleted) > trashDays*24*time.Hour {
			os.Remove(s.PhotoPath(t.Contact.ID))
			s.removeCardFiles(t.Contact)
			continue
		}
		keep = append(keep, t)
	}
	s.Trash = keep
}

// Groups lists every group name in use, plus empty ones the user created, sorted.
func (s *Store) Groups() []string {
	seen := map[string]string{}
	add := func(g string) {
		k := strings.ToLower(g)
		if _, ok := seen[k]; !ok && g != "" {
			seen[k] = g
		}
	}
	for _, c := range s.Contacts {
		for _, g := range c.Groups {
			add(g)
		}
	}
	for _, g := range s.ExtraGroups {
		add(g)
	}
	out := make([]string, 0, len(seen))
	for _, g := range seen {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return fold(out[i]) < fold(out[j]) })
	return out
}

// AddGroup records an empty group so it shows in the sidebar.
func (s *Store) AddGroup(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	for _, g := range s.Groups() {
		if strings.EqualFold(g, name) {
			return
		}
	}
	s.ExtraGroups = append(s.ExtraGroups, name)
}

// RenameGroup renames a group on every member.
func (s *Store) RenameGroup(old, name string) {
	name = strings.TrimSpace(name)
	for _, c := range s.Contacts {
		for i, g := range c.Groups {
			if strings.EqualFold(g, old) {
				c.Groups[i] = name
				c.Updated = time.Now()
			}
		}
		c.Tidy()
	}
	for i, g := range s.ExtraGroups {
		if strings.EqualFold(g, old) {
			s.ExtraGroups[i] = name
		}
	}
}

// RemoveGroup takes the group off every member; the contacts themselves stay.
func (s *Store) RemoveGroup(name string) {
	for _, c := range s.Contacts {
		keep := c.Groups[:0]
		for _, g := range c.Groups {
			if !strings.EqualFold(g, name) {
				keep = append(keep, g)
			}
		}
		if len(keep) != len(c.Groups) {
			c.Updated = time.Now()
		}
		c.Groups = keep
	}
	keep := s.ExtraGroups[:0]
	for _, g := range s.ExtraGroups {
		if !strings.EqualFold(g, name) {
			keep = append(keep, g)
		}
	}
	s.ExtraGroups = keep
}

// ---------- photos ----------

// PhotoPath is where a contact's photo lives.
func (s *Store) PhotoPath(id string) string { return filepath.Join(s.dir, "photos", id) }

// SetPhoto stores (or with nil, removes) a contact's photo.
func (s *Store) SetPhoto(c *Contact, img []byte) error {
	if len(img) == 0 { // kept aside (not deleted) so Undo can bring it back
		_ = os.Rename(s.PhotoPath(c.ID), s.PhotoPath(c.ID)+".removed")
		c.HasPhoto = false
		return nil
	}
	os.Remove(s.PhotoPath(c.ID) + ".removed")
	if err := writeFileAtomic(s.PhotoPath(c.ID), img, 0o600); err != nil {
		return err
	}
	c.HasPhoto = true
	return nil
}

// Photo reads a contact's photo, or nil.
func (s *Store) Photo(c *Contact) []byte {
	if !c.HasPhoto {
		return nil
	}
	b, err := os.ReadFile(s.PhotoPath(c.ID))
	if err != nil {
		b, err = os.ReadFile(s.PhotoPath(c.ID) + ".removed") // a removal that was undone
		if err != nil {
			return nil
		}
	}
	return b
}

// Undo history survives quitting: SaveUndo writes it next to the contacts, tied to the exact
// contacts file it applies to; if that file changed since (a restore, another copy), it's dropped.
type undoFile struct {
	Base  string     `json:"base"`
	Steps []undoStep `json:"steps"`
}

func (s *Store) undoPath() string { return filepath.Join(s.dir, "undo.json.gz") }

// SaveUndo writes the undo history (call when the app quits).
func (s *Store) SaveUndo() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.undo) == 0 {
		os.Remove(s.undoPath())
		return nil
	}
	var b bytes.Buffer
	z := gzip.NewWriter(&b)
	if err := json.NewEncoder(z).Encode(undoFile{Base: fmt.Sprintf("%x", s.lastHash), Steps: s.undo}); err != nil {
		return err
	}
	if err := z.Close(); err != nil {
		return err
	}
	return writeFileAtomic(s.undoPath(), b.Bytes(), 0o600)
}

func (s *Store) loadUndo(file []byte) {
	s.lastHash = sha256.Sum256(file)
	f, err := os.Open(s.undoPath())
	if err != nil {
		return
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		return
	}
	var u undoFile
	if json.NewDecoder(z).Decode(&u) != nil || u.Base != fmt.Sprintf("%x", s.lastHash) {
		return
	}
	s.undo = u.Steps
}

// Companies lists the companies in the book, most common first (for the editor's suggestions).
func (s *Store) Companies() []string {
	n := map[string]int{}
	spell := map[string]string{}
	for _, c := range s.Contacts {
		if k := fold(strings.TrimSpace(c.Company)); k != "" {
			n[k]++
			if spell[k] == "" {
				spell[k] = strings.TrimSpace(c.Company)
			}
		}
	}
	out := make([]string, 0, len(n))
	for k := range n {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if n[out[i]] != n[out[j]] {
			return n[out[i]] > n[out[j]]
		}
		return out[i] < out[j]
	})
	for i, k := range out {
		out[i] = spell[k]
	}
	return out
}
