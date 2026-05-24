// SPDX-License-Identifier: GPL-3.0-or-later
package pipeline

import (
	"github.com/autogame-17/scribe-studio/backend/scribe/models"
)

// pickInstalledModel picks the model whisper-cli will actually load.
// If `preferred` is non-empty and that model is installed, it wins —
// this is the user's manual choice from Settings → 转写 → 切换. Otherwise
// falls back to quality-first (large-v3-q5_0 > medium-q5_0 > medium >
// small > base > tiny). q5_0 quantized variants come first because they're
// ~4× smaller and ~2× faster than full precision with < 5% quality loss
// on Apple silicon Metal. Falls through all the way down so a fresh
// install still works with only the smallest one.
func pickInstalledModel(preferred string) string {
	if preferred != "" {
		if spec, ok := models.SpecByKey(preferred); ok {
			if inst, _ := models.IsInstalled(spec); inst {
				return preferred
			}
		}
		// preferred unknown or not installed — fall through to quality-first
		// rather than crash. UI is responsible for surfacing the stale prefs.
	}
	for _, key := range []string{"large-v3-q5_0", "medium-q5_0", "medium", "small", "base", "tiny"} {
		if spec, ok := models.SpecByKey(key); ok {
			if inst, _ := models.IsInstalled(spec); inst {
				return key
			}
		}
	}
	// Last resort: hand back "base" so the error surfaces to the UI
	// rather than silently using something nonsensical.
	return "base"
}
