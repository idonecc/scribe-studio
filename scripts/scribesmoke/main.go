//go:build scribesmoke

// Verify backend/scribe/transcribe + media + runtime in isolation
// from the Wails app. Runs against /tmp/scribe-test/sample.wav
// prepared by the outer shell script.
//
// Usage: go run -tags scribesmoke ./scripts/scribesmoke/main.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/autogame-17/scribe-studio/backend/scribe/transcribe"
)

func main() {
	audio := "/tmp/scribe-test/sample.wav"
	if _, err := os.Stat(audio); err != nil {
		fail("prerequisite missing: %v (run the smoke harness first)", err)
	}

	p := transcribe.LocalWhisperCpp{}
	fmt.Println("==> Transcribe")
	res, err := p.Transcribe(context.Background(), transcribe.Request{
		AudioPath: audio,
		Language:  "en",
		Model:     "tiny",
		OnProgress: func(frac float64, msg string) {
			if frac < 0 {
				fmt.Printf("   · %s\n", msg)
			} else {
				fmt.Printf("   · %.0f%% %s\n", frac*100, msg)
			}
		},
	})
	if err != nil {
		fail("transcribe: %v", err)
	}

	fmt.Printf("==> language=%s model=%s duration=%.2fs segments=%d\n",
		res.Language, res.Model, res.Duration, len(res.Segments))
	fmt.Printf("==> fullText:\n%s\n", res.FullText)

	raw, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println("==> result JSON:")
	fmt.Println(string(raw))
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "FAIL: "+format+"\n", a...)
	os.Exit(1)
}
