// SPDX-License-Identifier: GPL-3.0-or-later
// Package media wraps ffmpeg for audio extraction. The rest of the
// pipeline only cares about "give me a 16 kHz mono PCM WAV"; keep that
// assumption local to this package.
package media

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/autogame-17/scribe-studio/backend/scribe/runtime"
)

// ExtractAudio writes a 16 kHz mono WAV next to the video at `{name}.wav`
// (in tempDir if provided) and returns the output path. ffmpeg is invoked
// in overwrite mode so retries Just Work.
func ExtractAudio(ctx context.Context, inputPath, tempDir string) (string, error) {
	ffmpeg, err := runtime.BinaryPath("ffmpeg")
	if err != nil {
		return "", err
	}

	base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	dir := tempDir
	if dir == "" {
		var terr error
		if dir, terr = runtime.TempDir(); terr != nil {
			return "", terr
		}
	}
	outPath := filepath.Join(dir, base+".wav")

	cmd := exec.CommandContext(ctx, ffmpeg,
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-i", inputPath,
		"-vn",
		"-ac", "1",
		"-ar", "16000",
		"-c:a", "pcm_s16le",
		outPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg failed: %w\n%s", err, string(out))
	}
	return outPath, nil
}
