//go:build realsmoke

// SPDX-License-Identifier: GPL-3.0-or-later

// Verify scribe's full pipeline on a real audio/video file:
//   ffmpeg → whisper-cpp → parse → write SRT next to the video.
// Bypasses sphkit so it works while the Wails app is running.
//
// Usage:
//   go run -tags realsmoke ./scripts/realsmoke/main.go <video-or-audio-path> [model]
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/autogame-17/scribe-studio/backend/scribe/media"
	"github.com/autogame-17/scribe-studio/backend/scribe/transcribe"
)

func main() {
	if len(os.Args) < 2 {
		fail("usage: realsmoke <path> [model=base]")
	}
	in := os.Args[1]
	model := "base"
	if len(os.Args) >= 3 {
		model = os.Args[2]
	}

	if _, err := os.Stat(in); err != nil {
		fail("input not found: %v", err)
	}

	ctx := context.Background()

	// Stage 1: audio extract (skip if already WAV)
	var wav string
	if strings.EqualFold(filepath.Ext(in), ".wav") {
		wav = in
	} else {
		fmt.Println("==> ffmpeg: extracting audio")
		t := time.Now()
		w, err := media.ExtractAudio(ctx, in, "/tmp")
		if err != nil {
			fail("ffmpeg: %v", err)
		}
		wav = w
		fmt.Printf("   ok in %s -> %s\n", time.Since(t).Truncate(time.Millisecond), wav)
	}

	// Stage 2: transcribe
	fmt.Printf("==> whisper-cli model=%s\n", model)
	t := time.Now()
	res, err := transcribe.LocalWhisperCpp{}.Transcribe(ctx, transcribe.Request{
		AudioPath: wav,
		Language:  "auto",
		Model:     model,
		OnProgress: func(frac float64, msg string) {
			if frac >= 0 {
				fmt.Printf("   · %.0f%% %s\n", frac*100, msg)
			} else {
				fmt.Printf("   · %s\n", msg)
			}
		},
	})
	if err != nil {
		fail("transcribe: %v", err)
	}
	fmt.Printf("==> done in %s  lang=%s segments=%d duration=%.1fs\n",
		time.Since(t).Truncate(time.Millisecond), res.Language, len(res.Segments), res.Duration)

	// Stage 3: write SRT next to the source so playback clients
	// auto-load it. (Mirrors pipeline/save.go.)
	srtPath := writeSRT(in, res)
	fmt.Printf("==> wrote SRT: %s\n", srtPath)

	fmt.Println("----- FULL transcript -----")
	for i, seg := range res.Segments {
		fmt.Printf("[%02d] %s --> %s\n    %s\n",
			i+1, srtTime(seg.Start), srtTime(seg.End),
			strings.TrimSpace(seg.Text))
	}
	fmt.Println("---------------------------")
}

func writeSRT(videoPath string, r *transcribe.Result) string {
	base := strings.TrimSuffix(videoPath, filepath.Ext(videoPath))
	lang := r.Language
	if lang == "" {
		lang = "und"
	}
	out := base + "." + lang + ".srt"
	var b strings.Builder
	for i, seg := range r.Segments {
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n",
			i+1, srtTime(seg.Start), srtTime(seg.End),
			strings.TrimSpace(seg.Text))
	}
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		fail("write srt: %v", err)
	}
	return out
}

func srtTime(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	h := int(sec) / 3600
	m := (int(sec) % 3600) / 60
	s := int(sec) % 60
	ms := int((sec - float64(int(sec))) * 1000)
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "FAIL: "+format+"\n", a...)
	os.Exit(1)
}
