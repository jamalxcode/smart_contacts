package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
}

// TrashItem is a deleted contact, kept for 30 days.
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
	b, err := os.ReadFile(s.file())
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var f storeFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("contacts.json is damaged (%v); the daily backups are in %s", err, filepath.Join(dir, "backups"))
	}
	s.Contacts, s.Trash, s.ExtraGroups = f.Contacts, f.Trash, f.Groups
	s.purgeTrash(time.Now())
	return s, nil
}

func (s *Store) file() string { return filepath.Join(s.dir, "contacts.json") }

// Dir is where the data lives.
func (s *Store) Dir() string { return s.dir }

// Save writes atomically (temp file + rename) and makes the day's backup first.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backupLocked(time.Now())
	f := storeFile{Version: 1, Saved: time.Now(), Contacts: s.Contacts, Trash: s.Trash, Groups: s.ExtraGroups}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.file(), b, 0o600)
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
	for _, t := range s.Trash {
		os.Remove(s.PhotoPath(t.Contact.ID))
	}
	s.Trash = nil
}

func (s *Store) purgeTrash(now time.Time) {
	keep := s.Trash[:0]
	for _, t := range s.Trash {
		if now.Sub(t.Deleted) > trashDays*24*time.Hour {
			os.Remove(s.PhotoPath(t.Contact.ID))
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
	if len(img) == 0 {
		os.Remove(s.PhotoPath(c.ID))
		c.HasPhoto = false
		return nil
	}
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
		return nil
	}
	return b
}
