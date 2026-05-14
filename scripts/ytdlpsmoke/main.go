//go:build ytdlpsmoke

// SPDX-License-Identifier: GPL-3.0-or-later

// Validate the backend/scribe/external probe path against a known
// auth-free source (archive.org's Big Buck Bunny). Used as the v0.3
// e2e gate so we don't ship a Manager that can't even decode yt-dlp's
// JSON output.
//
// Usage: go run -tags ytdlpsmoke ./scripts/ytdlpsmoke/main.go
//
// The Probe function is unexported; we exercise it via the Manager's
// public Probe method, passing a context for the Wails-style call.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/autogame-17/scribe-studio/backend/scribe/external"
)

// fixedURL — archive.org's Big Buck Bunny is a venerable test target
// that doesn't require auth and won't disappear. If you need to swap
// it, pick anything served via a yt-dlp extractor that doesn't gate
// metadata behind a bot challenge.
const fixedURL = "https://archive.org/details/BigBuckBunny_124"

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// We don't need a real Manager (no AddURL / completion flow);
	// Probe only depends on the binary resolver and the ctx. But the
	// only way to call probe() is via Manager — so spin up a throwaway
	// one with a tmp StateDir (the manager only writes on AddURL).
	mgr, err := external.NewManager(ctx, os.TempDir())
	if err != nil {
		fail("manager: %v", err)
	}

	fmt.Printf("==> probing %s\n", fixedURL)
	res, err := mgr.Probe(fixedURL, "")
	if err != nil {
		fail("probe: %v", err)
	}

	if res.Title == "" {
		fail("probe returned empty title — yt-dlp output schema may have shifted")
	}
	if len(res.Formats) == 0 {
		fail("probe returned zero formats — pickFormats() filter may be too strict")
	}

	fmt.Printf("==> title=%q\n", res.Title)
	fmt.Printf("==> site=%q duration=%.1fs\n", res.Site, res.Duration)
	fmt.Printf("==> %d formats:\n", len(res.Formats))
	for _, f := range res.Formats {
		fmt.Printf("    · %s (%s, %dp)\n", f.ID, f.Label, f.Height)
	}

	raw, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println("==> ProbeResult JSON:")
	fmt.Println(string(raw))
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "FAIL: "+format+"\n", a...)
	os.Exit(1)
}
