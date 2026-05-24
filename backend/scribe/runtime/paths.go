// SPDX-License-Identifier: GPL-3.0-or-later
// Package runtime centralises filesystem + binary path resolution so the
// transcribe/media/pipeline packages never hard-code absolute paths.
package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
)

const appName = "Scribe"

// AppSupportDir is the per-user data root:
//   - macOS:   ~/Library/Application Support/Scribe
//   - Windows: %APPDATA%\Scribe
//   - Linux:   ~/.config/Scribe
// Created on demand (callers don't pre-mkdir).
func AppSupportDir() (string, error) {
	var root string
	switch goruntime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, "Library", "Application Support", appName)
	case "windows":
		if v := os.Getenv("APPDATA"); v != "" {
			root = filepath.Join(v, appName)
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			root = filepath.Join(home, "AppData", "Roaming", appName)
		}
	default:
		cfg, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(cfg, appName)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	return root, nil
}

// SubDir returns an AppSupportDir subpath, creating it on the way.
func SubDir(parts ...string) (string, error) {
	root, err := AppSupportDir()
	if err != nil {
		return "", err
	}
	p := filepath.Join(append([]string{root}, parts...)...)
	if err := os.MkdirAll(p, 0o755); err != nil {
		return "", err
	}
	return p, nil
}

// ModelsDir is where whisper ggml models live. Independent from AppSupportDir
// so all model files can live under a unified ~/models/<App> convention,
// shared with other model-using tools (omlx, MyType, etc.). Override with
// SCRIBE_MODELS_DIR env var if needed.
func ModelsDir() (string, error) {
	if v := os.Getenv("SCRIBE_MODELS_DIR"); v != "" {
		if err := os.MkdirAll(v, 0o755); err != nil {
			return "", err
		}
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	p := filepath.Join(home, "models", appName)
	if err := os.MkdirAll(p, 0o755); err != nil {
		return "", err
	}
	return p, nil
}

// TranscriptsDir is where per-task JSON transcripts are persisted.
func TranscriptsDir() (string, error) { return SubDir("transcripts") }

// StateDir holds pipeline state snapshots (crash-safe).
func StateDir() (string, error) { return SubDir("state") }

// TempDir returns a private scratch area for extracted audio; callers are
// responsible for cleaning up files they create.
func TempDir() (string, error) { return SubDir("tmp") }

// MustAppSupportDir panics if the directory can't be created. Use only in
// init-adjacent paths where failure is unrecoverable anyway.
func MustAppSupportDir() string {
	p, err := AppSupportDir()
	if err != nil {
		panic(fmt.Errorf("scribe: %w", err))
	}
	return p
}
