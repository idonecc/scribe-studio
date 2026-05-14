// SPDX-License-Identifier: GPL-3.0-or-later
// Package transcribe defines the engine-agnostic contract the pipeline
// speaks to. LocalWhisperCpp is the default implementation; cloud
// providers (OpenAI, Groq) slot into the same Provider interface.
package transcribe

import (
	"context"
)

// Segment is one timestamped chunk of recognized speech. whisper-cpp's
// JSON output maps 1:1 here.
type Segment struct {
	Start float64 `json:"start"` // seconds from t=0
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

// Result collects everything the pipeline saves and renders.
type Result struct {
	Language string    `json:"language"`
	Model    string    `json:"model"`    // e.g. "whisper-cpp:base"
	Segments []Segment `json:"segments"`
	FullText string    `json:"fullText"`
	Duration float64   `json:"duration"` // audio length in seconds
}

// ProgressFn is called from the provider as work progresses. The
// argument is a 0..1 fraction; msg is an optional human-readable tag
// (e.g. "transcribing", "loading model").
type ProgressFn func(fraction float64, msg string)

// Request captures everything a Transcribe call needs. Model is a
// provider-specific identifier — for LocalWhisperCpp it's "base" /
// "small" / "medium", mapped to ggml-<model>.bin on disk.
type Request struct {
	AudioPath  string
	Language   string // "zh", "en", "auto"
	Model      string
	OnProgress ProgressFn
}

// Provider is the only abstraction the rest of the app consumes.
type Provider interface {
	Name() string
	Transcribe(ctx context.Context, req Request) (*Result, error)
}
