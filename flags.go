package main

import (
	"embed"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"github.com/nyaruka/phonenumbers"
)

// Country flags beside phone numbers. The toolkit can't draw colour emoji, so the flags are
// small pictures (from the MIT-licensed flag-icons set, flags/LICENSE-flag-icons.txt), picked
// from the number's country code: +965 → Kuwait, +44 → United Kingdom.

//go:embed flags/*.png
var flagFS embed.FS

var (
	flagMu    sync.Mutex
	flagCache = map[string]fyne.Resource{}
)

// PhoneRegion is the two-letter country of a number ("KW"); local numbers take the home
// country. "" when it can't be told.
func PhoneRegion(number, defCC string) string {
	n := NormalizePhone(number, defCC)
	if !strings.HasPrefix(n, "+") {
		return ""
	}
	if p, err := phonenumbers.Parse(n, ""); err == nil {
		if r := phonenumbers.GetRegionCodeForNumber(p); r != "" && r != "ZZ" {
			return r
		}
		if r := phonenumbers.GetRegionCodeForCountryCode(int(p.GetCountryCode())); r != "" && r != "ZZ" {
			return r
		}
	}
	return ""
}

// flagResource is the flag picture for a country code like "KW" (nil if there's none).
func flagResource(region string) fyne.Resource {
	region = strings.ToLower(region)
	if len(region) != 2 {
		return nil
	}
	flagMu.Lock()
	defer flagMu.Unlock()
	if r, ok := flagCache[region]; ok {
		return r
	}
	b, err := flagFS.ReadFile("flags/" + region + ".png")
	var r fyne.Resource
	if err == nil {
		r = fyne.NewStaticResource("flag-"+region+".png", b)
	}
	flagCache[region] = r
	return r
}

// newFlag is a small flag image (hidden when there's no flag); setFlag updates it for a number.
func newFlag() *canvas.Image {
	img := &canvas.Image{FillMode: canvas.ImageFillContain}
	img.SetMinSize(fyne.NewSize(22, 16))
	img.Hide()
	return img
}

func setFlag(img *canvas.Image, number, defCC string) {
	r := flagResource(PhoneRegion(number, defCC))
	if r == nil {
		img.Resource = nil
		img.Hide()
		return
	}
	img.Resource = r
	img.Show()
	img.Refresh()
}
