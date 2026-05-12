package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/autogame-17/scribe-studio/backend/scribe/runtime"
	"github.com/autogame-17/scribe-studio/backend/scribe/transcribe"
)

// saveTranscriptJSON writes the structured transcript to
// AppSupport/transcripts/<taskID>.json. That file is our source of
// truth; the editor reads it, the SRT export rebuilds from it.
func saveTranscriptJSON(taskID string, r *transcribe.Result) (string, error) {
	dir, err := runtime.TranscriptsDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, taskID+".json")
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// saveSRT writes `{video}.zh.srt` next to the original video so
// players (IINA, VLC, mpv) auto-load it.
func saveSRT(videoPath string, r *transcribe.Result) (string, error) {
	base := strings.TrimSuffix(videoPath, filepath.Ext(videoPath))
	lang := r.Language
	if lang == "" {
		lang = "und"
	}
	path := base + "." + lang + ".srt"
	var b strings.Builder
	for i, seg := range r.Segments {
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n",
			i+1,
			formatSRTTime(seg.Start),
			formatSRTTime(seg.End),
			strings.TrimSpace(seg.Text),
		)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func formatSRTTime(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	h := int(sec) / 3600
	m := (int(sec) % 3600) / 60
	s := int(sec) % 60
	ms := int((sec - float64(int(sec))) * 1000)
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}
