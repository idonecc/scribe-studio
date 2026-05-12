package scribe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/autogame-17/scribe-studio/backend/scribe/models"
	"github.com/autogame-17/scribe-studio/backend/scribe/pipeline"
	"github.com/autogame-17/scribe-studio/backend/scribe/transcribe"
)

// Re-exported so Wails' TypeScript binding generator surfaces them
// under the scribe namespace rather than pipeline/models/transcribe.
type TranscribeJob = pipeline.Job
type TranscribeStage = pipeline.Stage

type ModelSummary struct {
	Key       string `json:"key"`
	Filename  string `json:"filename"`
	URL       string `json:"url"`
	Bytes     int64  `json:"bytes"`
	Label     string `json:"label"`
	Installed bool   `json:"installed"`
}

type TranscribeSettings struct {
	AutoEnabled bool   `json:"autoEnabled"`
	Model       string `json:"model"` // key: "base" / "small" / "medium"
	Language    string `json:"language"`
}

// ListTranscripts returns all known transcription jobs, newest first.
// Used by the Transcripts page and to hydrate TranscribeProgress rows
// on the Downloads page.
func (a *App) ListTranscripts() []TranscribeJob {
	a.mu.Lock()
	p := a.pipeline
	a.mu.Unlock()
	if p == nil {
		return nil
	}
	return p.ListJobs()
}

// GetTranscript returns the full Whisper Result for a task (segments +
// language + duration). The editor consumes this.
func (a *App) GetTranscript(taskID string) (*transcribe.Result, error) {
	a.mu.Lock()
	p := a.pipeline
	a.mu.Unlock()
	if p == nil {
		return nil, errors.New("pipeline not initialised")
	}
	job, ok := p.ListJobsMap()[taskID]
	if !ok {
		return nil, fmt.Errorf("no such task: %s", taskID)
	}
	if job.TranscriptPath == "" {
		return nil, errors.New("transcript not ready")
	}
	raw, err := os.ReadFile(job.TranscriptPath)
	if err != nil {
		return nil, err
	}
	var r transcribe.Result
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// RetryTranscribe pushes a task back through the pipeline. No-op if
// the pipeline isn't running yet.
func (a *App) RetryTranscribe(taskID string) error {
	a.mu.Lock()
	p := a.pipeline
	a.mu.Unlock()
	if p == nil {
		return errors.New("pipeline not initialised")
	}
	p.Retry(taskID)
	return nil
}

// SetAutoTranscribe toggles the watcher's enqueue-on-new-download
// behaviour. Manual Retry always works regardless.
func (a *App) SetAutoTranscribe(enabled bool) {
	a.mu.Lock()
	p := a.pipeline
	a.mu.Unlock()
	if p != nil {
		p.SetAutoEnabled(enabled)
	}
}

func (a *App) GetTranscribeSettings() TranscribeSettings {
	a.mu.Lock()
	p := a.pipeline
	a.mu.Unlock()
	auto := true
	if p != nil {
		auto = p.AutoEnabled()
	}
	return TranscribeSettings{
		AutoEnabled: auto,
		Model:       "base",
		Language:    "auto",
	}
}

// ListModels reports every known whisper model + whether it's on disk.
func (a *App) ListModels() []ModelSummary {
	out := make([]ModelSummary, 0, len(models.Known))
	for _, spec := range models.Known {
		inst, _ := models.IsInstalled(spec)
		out = append(out, ModelSummary{
			Key:       spec.Key,
			Filename:  spec.Filename,
			URL:       spec.URL,
			Bytes:     spec.Bytes,
			Label:     spec.Label,
			Installed: inst,
		})
	}
	return out
}

// DownloadModel starts an async download for the given model key and
// emits "model:progress" + "model:done" events to the frontend.
// Returns immediately once the download goroutine is spawned.
func (a *App) DownloadModel(key string) error {
	spec, ok := models.SpecByKey(key)
	if !ok {
		return fmt.Errorf("unknown model: %s", key)
	}
	ctx := a.ctx
	go func() {
		err := models.Download(context.Background(), spec, func(frac float64, msg string) {
			runtime.EventsEmit(ctx, "model:progress", map[string]any{
				"key":      spec.Key,
				"fraction": frac,
				"message":  msg,
			})
		})
		payload := map[string]any{"key": spec.Key}
		if err != nil {
			payload["error"] = err.Error()
			log.Printf("scribe: model download failed: %v", err)
		}
		runtime.EventsEmit(ctx, "model:done", payload)
	}()
	return nil
}
