// SPDX-License-Identifier: GPL-3.0-or-later
package pipeline

import (
	"github.com/autogame-17/scribe-studio/backend/scribe/models"
)

// pickInstalledModel picks the best model actually on disk. Preference
// order is quality-first (medium > small > base > tiny), but we fall
// back all the way down so the pipeline works on a freshly-installed
// app where the user has only downloaded the smallest one.
func pickInstalledModel() string {
	for _, key := range []string{"medium", "small", "base", "tiny"} {
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
