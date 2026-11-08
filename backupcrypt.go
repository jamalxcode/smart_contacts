package main

import (
	"archive/zip"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Password-protected backups: the backup zip, encrypted with AES-256-GCM under a key made from
// the password (PBKDF2-SHA256, 600,000 rounds). File layout: "SCBK1" | salt (16) | nonce (12) |
// ciphertext. Without the password the file is useless; the password is never stored.

const backupMagic = "SCBK1"

func deriveKey(password string, salt []byte) ([]byte, error) {
	return pbkdf2.Key(sha256.New, password, salt, 600000, 32)
}

// EncryptBackup seals zip bytes with a password.
func EncryptBackup(plain []byte, password string) ([]byte, error) {
	if password == "" {
		return nil, errors.New("no password")
	}
	salt := make([]byte, 16)
	nonce := make([]byte, 12)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	key, err := deriveKey(password, salt)
	if err != nil {
		return nil, err
	}
	blk, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(blk)
	if err != nil {
		return nil, err
	}
	out := append([]byte(backupMagic), salt...)
	out = append(out, nonce...)
	return gcm.Seal(out, nonce, plain, []byte(backupMagic)), nil
}

// IsEncryptedBackup tells a protected backup from a plain zip.
func IsEncryptedBackup(b []byte) bool { return bytes.HasPrefix(b, []byte(backupMagic)) }

// DecryptBackup opens a protected backup.
func DecryptBackup(b []byte, password string) ([]byte, error) {
	if !IsEncryptedBackup(b) || len(b) < len(backupMagic)+16+12+16 {
		return nil, errors.New("this isn't a Smart Contacts backup")
	}
	salt := b[len(backupMagic) : len(backupMagic)+16]
	nonce := b[len(backupMagic)+16 : len(backupMagic)+28]
	key, err := deriveKey(password, salt)
	if err != nil {
		return nil, err
	}
	blk, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(blk)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonce, b[len(backupMagic)+28:], []byte(backupMagic))
	if err != nil {
		return nil, errors.New("wrong password, or the backup is damaged")
	}
	return plain, nil
}

// zipDataBytes is the backup zip in memory (the OpenRouter key left out).
func zipDataBytes(src string) ([]byte, error) {
	tmp, err := os.MkdirTemp("", "sc-backup-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	p := filepath.Join(tmp, "b.zip")
	if err := zipDataDir(src, p); err != nil {
		return nil, err
	}
	return os.ReadFile(p)
}

// RestoreFiles copies a backup's photos, business cards, calendar and history into dir (files
// already there stay) and returns the path of its contacts.json, extracted to a temporary file
// for Store.RestoreFile. Settings aren't restored.
func RestoreFiles(zipBytes []byte, dir string) (string, func(), error) {
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return "", nil, errors.New("this backup can't be opened (not a Smart Contacts backup?)")
	}
	tmp, err := os.MkdirTemp("", "sc-restore-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { os.RemoveAll(tmp) }
	contacts := ""
	for _, f := range zr.File {
		name := filepath.ToSlash(filepath.Clean(f.Name))
		if strings.HasPrefix(name, "..") || strings.HasPrefix(name, "/") || f.FileInfo().IsDir() {
			continue // never write outside the data folder
		}
		var dest string
		switch {
		case name == "contacts.json":
			dest = filepath.Join(tmp, "contacts.json")
			contacts = dest
		case strings.HasPrefix(name, "photos/"), strings.HasPrefix(name, "cards/"):
			dest = filepath.Join(dir, filepath.FromSlash(name))
			if _, err := os.Stat(dest); err == nil {
				continue
			}
		case name == "calendar.json", name == "activity.json":
			dest = filepath.Join(dir, name)
		default:
			continue
		}
		rc, err := f.Open()
		if err != nil {
			cleanup()
			return "", nil, err
		}
		b, err := io.ReadAll(io.LimitReader(rc, 1<<30))
		rc.Close()
		if err != nil {
			cleanup()
			return "", nil, err
		}
		_ = os.MkdirAll(filepath.Dir(dest), 0o700)
		if err := writeFileAtomic(dest, b, 0o600); err != nil {
			cleanup()
			return "", nil, err
		}
	}
	if contacts == "" {
		cleanup()
		return "", nil, errors.New("this backup has no contacts in it")
	}
	return contacts, cleanup, nil
}
