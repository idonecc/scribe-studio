// SPDX-License-Identifier: GPL-3.0-or-later
// Preferences for the transcription engine. Currently only tracks the
// user-selected active Whisper model — picker falls back to quality-first
// auto-detect when ActiveModel is empty or refers to an uninstalled key.
// Persisted to AppSupport/Scribe/transcribe-preferences.json with mode 0644
// (no secrets here, no need to lock down).
package transcribe

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/autogame-17/scribe-studio/backend/scribe/runtime"
)

type Preferences struct {
	// ActiveModel is the user's manually-picked Whisper model key
	// ("tiny" / "small" / "medium-q5_0" / "large-v3-q5_0" / ...). Empty
	// means "no manual choice yet, let the picker pick quality-first".
	ActiveModel string `json:"activeModel"`
}

func preferencesPath() (string, error) {
	dir, err := runtime.AppSupportDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "transcribe-preferences.json"), nil
}

// LoadPreferences reads transcribe-preferences.json. Missing file or
// unreadable file silently returns the zero value — a fresh install
// should boot with auto-pick rather than wedge on a missing config.
func LoadPreferences() Preferences {
	path, err := preferencesPath()
	if err != nil {
		return Preferences{}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Preferences{}
	}
	var p Preferences
	_ = json.Unmarshal(raw, &p)
	return p
}

// SavePreferences writes atomically via tmpfile + rename so a crash
// mid-write doesn't leave a half-truncated JSON the next load chokes on.
func SavePreferences(p Preferences) error {
	path, err := preferencesPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
