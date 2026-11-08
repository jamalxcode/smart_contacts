package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Config is the one settings file: <data dir>/config.json (readable, permission 0600).
// The OpenRouter key lives here on purpose, not in the macOS Keychain, so the app never
// asks for the Mac password.
type Config struct {
	WindowW       float32 `json:"windowW,omitempty"`
	WindowH       float32 `json:"windowH,omitempty"`
	LayoutVersion int     `json:"layoutVersion,omitempty"`
	SortByLast    bool    `json:"sortByLast,omitempty"`
	CountryCode   string  `json:"countryCode"`
	OpenRouterKey string  `json:"openRouterKey,omitempty"`
	// AllowCloudAI lets Smart Add send pasted text to OpenRouter. Off by default: with it off,
	// nothing about any contact ever leaves this Mac.
	AllowCloudAI bool      `json:"allowCloudAI,omitempty"`
	AIModel      string    `json:"aiModel,omitempty"`
	LastGroup    string    `json:"lastGroup,omitempty"`
	LastCleanup  time.Time `json:"lastCleanup,omitempty"`
	LastDedupe   time.Time `json:"lastDedupe,omitempty"`
	// Activity: read Messages, calls and Mail when the app opens (on after the first good read).
	AutoActivity bool     `json:"autoActivity,omitempty"`
	IgnoredKeys  []string `json:"ignoredKeys,omitempty"` // "not a contact" numbers/emails
	// Smart groups are saved searches shown in the sidebar.
	SmartGroups []SmartGroup `json:"smartGroups,omitempty"`
	// Reminders: birthdays and people to reach out to, once a day.
	NoNotify     bool   `json:"noNotify,omitempty"`
	LastNotified string `json:"lastNotified,omitempty"`
}

const layoutVersion = 1

func configPath(dir string) string { return filepath.Join(dir, "config.json") }

// LoadConfig reads the settings, with defaults for anything missing.
func LoadConfig(dir string) *Config {
	c := &Config{CountryCode: "965", AIModel: DefaultAIModel}
	if b, err := os.ReadFile(configPath(dir)); err == nil {
		_ = json.Unmarshal(b, c)
	}
	if c.CountryCode == "" {
		c.CountryCode = "965"
	}
	if c.AIModel == "" {
		c.AIModel = DefaultAIModel
	}
	if c.LayoutVersion != layoutVersion {
		c.WindowW, c.WindowH = 0, 0
		c.LayoutVersion = layoutVersion
	}
	return c
}

// Save writes the settings.
func (c *Config) Save(dir string) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	_ = os.MkdirAll(dir, 0o700)
	return writeFileAtomic(configPath(dir), b, 0o600)
}

// SmartGroup is a saved search ("company:boubyan has:email").
type SmartGroup struct {
	Name  string `json:"name"`
	Query string `json:"query"`
}
