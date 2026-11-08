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

// CardImage is a business card photo or screenshot kept with a contact.
type CardImage struct {
	File  string    `json:"file"`           // name inside <data dir>/cards
	Added time.Time `json:"added"`          //
	Note  string    `json:"note,omitempty"` // "added this contact", "updated this contact"
}

func (s *Store) cardsDir() string { return filepath.Join(s.dir, "cards") }

// CardImagePath is where a card image is stored.
func (s *Store) CardImagePath(file string) string {
	return filepath.Join(s.cardsDir(), filepath.Base(file))
}

// AddCardImage stores an image (JPEG or PNG; convert HEIC first) with the contact.
func (s *Store) AddCardImage(c *Contact, img []byte, note string) error {
	if !CanDecodeImage(img) {
		return errors.New("that picture couldn't be read — use a JPEG, PNG or HEIC image")
	}
	if err := os.MkdirAll(s.cardsDir(), 0o700); err != nil {
		return err
	}
	img = ShrinkPhoto(img, 1800) // keep cards readable but not camera-sized
	name := fmt.Sprintf("%s-%s.jpg", c.ID, time.Now().Format("20060102-150405.000"))
	if err := writeFileAtomic(s.CardImagePath(name), img, 0o600); err != nil {
		return err
	}
	c.CardImages = append(c.CardImages, CardImage{File: name, Added: time.Now(), Note: note})
	return nil
}

// RemoveCardImage deletes one card image from a contact.
func (s *Store) RemoveCardImage(c *Contact, file string) {
	keep := c.CardImages[:0]
	for _, ci := range c.CardImages {
		if ci.File == file {
			os.Remove(s.CardImagePath(file))
			continue
		}
		keep = append(keep, ci)
	}
	c.CardImages = keep
}

// removeCardFiles deletes every card image of a contact (used when the Trash is emptied).
func (s *Store) removeCardFiles(c *Contact) {
	for _, ci := range c.CardImages {
		os.Remove(s.CardImagePath(ci.File))
	}
}

// ToJPEG converts an image macOS can read but this app can't (HEIC from an iPhone, TIFF…) to
// JPEG with the built-in sips tool. JPEG and PNG come back unchanged.
func ToJPEG(data []byte, name string) ([]byte, error) {
	if CanDecodeImage(data) {
		return data, nil
	}
	if runtime.GOOS != "darwin" {
		return nil, errors.New("that picture couldn't be read — use a JPEG or PNG image")
	}
	dir, err := os.MkdirTemp("", "smartcontacts-img-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	in := filepath.Join(dir, "in"+strings.ToLower(filepath.Ext(name)))
	out := filepath.Join(dir, "out.jpg")
	if err := os.WriteFile(in, data, 0o600); err != nil {
		return nil, err
	}
	if b, err := exec.Command("sips", "-s", "format", "jpeg", in, "--out", out).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("couldn't convert that picture: %s", strings.TrimSpace(string(b)))
	}
	return os.ReadFile(out)
}

// IsImageFile reports whether a file name looks like a picture.
func IsImageFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".heic", ".heif", ".tif", ".tiff", ".gif", ".webp", ".bmp":
		return true
	}
	return false
}

// ocrScript reads text from an image with the Mac's own Vision framework (no internet).
// It asks for Arabic and English, and falls back to English on Macs without Arabic support.
const ocrScript = `ObjC.import('Foundation');
ObjC.import('Vision');
function attempt(url, langs) {
	const handler = $.VNImageRequestHandler.alloc.initWithURLOptions(url, $.NSDictionary.dictionary);
	const req = $.VNRecognizeTextRequest.alloc.init;
	req.recognitionLevel = 0;
	req.usesLanguageCorrection = true;
	if (langs) { req.recognitionLanguages = $(langs); }
	if (!handler.performRequestsError($([req]), $())) { return null; }
	const results = req.results;
	const lines = [];
	for (let i = 0; i < results.count; i++) {
		const cand = results.objectAtIndex(i).topCandidates(1);
		if (cand.count > 0) { lines.push(cand.objectAtIndex(0).string.js); }
	}
	return lines.join("\n");
}
function run(argv) {
	const url = $.NSURL.fileURLWithPath(argv[0]);
	let text = null;
	try { text = attempt(url, ["ar-SA", "en-US"]); } catch (e) { text = null; }
	if (text === null || text === "") {
		try { text = attempt(url, ["en-US"]); } catch (e) { text = null; }
	}
	if (text === null) { throw new Error("text recognition failed"); }
	return text;
}`

// ReadCardText recognises the text on a business card image, on this Mac.
func ReadCardText(img []byte) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", errors.New("reading text from images needs macOS")
	}
	dir, err := os.MkdirTemp("", "smartcontacts-ocr-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	imgPath := filepath.Join(dir, "card.jpg")
	scriptPath := filepath.Join(dir, "ocr.js")
	if err := os.WriteFile(imgPath, img, 0o600); err != nil {
		return "", err
	}
	if err := os.WriteFile(scriptPath, []byte(ocrScript), 0o600); err != nil {
		return "", err
	}
	cmd := exec.Command("osascript", "-l", "JavaScript", scriptPath, imgPath)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("couldn't read the card: %s", strings.TrimSpace(errb.String()))
	}
	text := strings.TrimSpace(out.String())
	if text == "" {
		return "", errors.New("no text found on that image — try a sharper, straighter photo")
	}
	return text, nil
}
