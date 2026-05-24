// SPDX-License-Identifier: GPL-3.0-or-later
// Package pipeline is the glue that turns a finished download into a
// saved transcript. It runs one worker so local Whisper doesn't tank
// the machine when a batch of videos finishes at once.
package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/autogame-17/scribe-studio/backend/scribe/logbus"
	"github.com/autogame-17/scribe-studio/backend/scribe/media"
	"github.com/autogame-17/scribe-studio/backend/scribe/proofread"
	screbuntime "github.com/autogame-17/scribe-studio/backend/scribe/runtime"
	"github.com/autogame-17/scribe-studio/backend/scribe/transcribe"
	"wx_channel/pkg/sphkit"
)

// Stage names mirror what the UI pill shows.
type Stage string

const (
	StagePending      Stage = "pending"
	StageExtracting   Stage = "extracting"
	StageTranscribing Stage = "transcribing"
	StageSaving       Stage = "saving"
	StageDone         Stage = "done"
	StageFailed       Stage = "failed"
)

// Source identifies where the original video came from so Retry and
// the watcher know where to look for fresh metadata.
type Source string

const (
	// SourceWxChannel covers everything that came through the
	// wx_channel MITM proxy (i.e. sphkit/gopeed BoltDB tasks).
	SourceWxChannel Source = "wx_channel"
	// SourceExternal covers yt-dlp-driven downloads (YouTube,
	// Bilibili, etc.). VideoPath is authoritative — there's no
	// sphkit to re-hydrate from.
	SourceExternal Source = "external"
)

// Job is the persisted record tracked per download. It's what the UI
// receives via the "transcribe:job" event and what ListTranscripts
// returns.
type Job struct {
	TaskID         string  `json:"taskID"`
	Title          string  `json:"title"`
	VideoPath      string  `json:"videoPath"`
	// Source is empty for legacy records — Retry treats those as
	// wx_channel for backwards compatibility with state files
	// written before the multi-source split.
	Source         Source  `json:"source,omitempty"`
	Stage          Stage   `json:"stage"`
	Progress       float64 `json:"progress"` // 0..1, -1 if indeterminate
	ProgressMsg    string  `json:"progressMsg,omitempty"`
	TranscriptPath string  `json:"transcriptPath,omitempty"` // JSON in AppSupport
	SRTPath        string  `json:"srtPath,omitempty"`        // next to the video
	Error          string  `json:"error,omitempty"`
	Retries        int     `json:"retries"`
	Model          string  `json:"model,omitempty"`
	Language       string  `json:"language,omitempty"`
	Duration       float64 `json:"duration,omitempty"`
	CreatedAt      string  `json:"createdAt"`
	UpdatedAt      string  `json:"updatedAt"`
}

// KitProvider is injected so the pipeline doesn't own the sphkit
// Instance's lifecycle (App does). Returns nil while proxy is stopped.
type KitProvider func() *sphkit.Instance

// Pipeline runs a watcher + single worker for the lifetime of the app.
type Pipeline struct {
	ctx      context.Context
	kitFn    KitProvider
	provider transcribe.Provider
	glossary *proofread.Glossary

	state *State

	jobs   chan string      // taskIDs queued for processing
	inFlight map[string]struct{}
	mu     sync.Mutex

	stopCh chan struct{}
	wg     sync.WaitGroup

	autoEnabled bool
}

func New(ctx context.Context, kitFn KitProvider) (*Pipeline, error) {
	st, err := LoadState()
	if err != nil {
		return nil, err
	}
	g, err := proofread.Load()
	if err != nil {
		// Non-fatal: missing glossary just means no deterministic
		// pass, the pipeline still produces raw whisper output.
		g = nil
	}
	return &Pipeline{
		ctx:         ctx,
		kitFn:       kitFn,
		provider:    transcribe.DefaultProvider(),
		glossary:    g,
		state:       st,
		jobs:        make(chan string, 32),
		inFlight:    map[string]struct{}{},
		stopCh:      make(chan struct{}),
		autoEnabled: true,
	}, nil
}

// Glossary exposes the shared glossary so App can wire up its CRUD
// bindings without another Load().
func (p *Pipeline) Glossary() *proofread.Glossary { return p.glossary }

// SetAutoEnabled toggles whether the watcher enqueues new downloads
// automatically. Manual re-transcribe still works when disabled.
func (p *Pipeline) SetAutoEnabled(v bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.autoEnabled = v
}

func (p *Pipeline) AutoEnabled() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.autoEnabled
}

// Start launches the watcher + worker goroutines. Safe to call once.
func (p *Pipeline) Start() {
	p.wg.Add(2)
	go p.runWatcher()
	go p.runWorker()
}

// Stop cleans up goroutines. Fire-and-forget at Wails OnShutdown.
func (p *Pipeline) Stop() {
	close(p.stopCh)
	p.wg.Wait()
}

// ListJobs returns a copy of the current job map, sorted by update time
// desc, so the UI can render without holding the pipeline's lock.
func (p *Pipeline) ListJobs() []Job {
	return p.state.Snapshot()
}

// ListJobsMap returns a map keyed by taskID for O(1) lookups from App.
func (p *Pipeline) ListJobsMap() map[string]Job {
	p.state.mu.Lock()
	defer p.state.mu.Unlock()
	out := make(map[string]Job, len(p.state.Jobs))
	for k, v := range p.state.Jobs {
		out[k] = v
	}
	return out
}

// Retry forces a re-transcribe for a known task. For wx_channel
// jobs we re-fetch from sphkit (so records persisted by an older
// parser auto-correct). External jobs already carry their canonical
// VideoPath/Title from the yt-dlp manager — we just reset the Stage
// and re-queue.
func (p *Pipeline) Retry(taskID string) {
	if existing, ok := p.state.Get(taskID); ok && existing.Source == SourceExternal {
		existing.Stage = StagePending
		existing.Error = ""
		existing.Progress = 0
		existing.ProgressMsg = ""
		existing.UpdatedAt = nowISO()
		p.upsertJob(existing)
	} else {
		p.hydrateJob(taskID)
	}
	p.enqueue(taskID)
}

// EnqueueExternal registers a Job for an external (yt-dlp) download
// that just finished, then schedules it for transcribe. Called from
// the external Manager's completion callback. Safe to call multiple
// times for the same taskID (idempotent — second call just bumps
// timestamps and re-queues if not already in-flight).
func (p *Pipeline) EnqueueExternal(taskID, title, videoPath string) {
	if taskID == "" || videoPath == "" {
		return
	}
	job := Job{
		TaskID:    taskID,
		Title:     title,
		VideoPath: videoPath,
		Source:    SourceExternal,
		Stage:     StagePending,
		CreatedAt: nowISO(),
		UpdatedAt: nowISO(),
	}
	if existing, ok := p.state.Get(taskID); ok {
		// Preserve existing CreatedAt + Retries; refresh path/title.
		job.CreatedAt = existing.CreatedAt
		job.Retries = existing.Retries
	}
	p.state.MarkSeen(taskID)
	p.upsertJob(job)
	p.enqueue(taskID)
}

// enqueue pushes a taskID onto the worker channel. If the channel is
// full we fall back to a goroutine that blocks until either a slot
// opens or the pipeline shuts down, so a flurry of retries can't
// silently drop work.
func (p *Pipeline) enqueue(taskID string) {
	select {
	case p.jobs <- taskID:
	default:
		go func() {
			select {
			case p.jobs <- taskID:
			case <-p.stopCh:
			}
		}()
	}
}

// hydrateJob looks up the task in sphkit and creates or refreshes the
// Job entry so processOne has up-to-date path/title to act on. If the
// task can't be found (e.g. proxy stopped, task purged) any existing
// Job is left untouched.
func (p *Pipeline) hydrateJob(taskID string) {
	kit := p.kitFn()
	if kit == nil {
		return
	}
	// Search across all tasks — the API doesn't expose a direct by-id
	// lookup, so we paginate until found. 200 per page covers the vast
	// majority of real users.
	page, err := kit.ListTasks("all", 1, 200)
	if err != nil {
		return
	}
	for _, t := range page.Tasks {
		if t.ID != taskID {
			continue
		}
		p.state.MarkSeen(t.ID)
		videoPath := filepath.Join(t.Path, t.Filename)
		if existing, ok := p.state.Get(taskID); ok {
			existing.Title = t.Title
			existing.VideoPath = videoPath
			existing.Stage = StagePending
			existing.Error = ""
			existing.Progress = 0
			existing.ProgressMsg = ""
			existing.UpdatedAt = nowISO()
			p.upsertJob(existing)
			return
		}
		p.upsertJob(Job{
			TaskID:    t.ID,
			Title:     t.Title,
			VideoPath: videoPath,
			Source:    SourceWxChannel,
			Stage:     StagePending,
			CreatedAt: nowISO(),
			UpdatedAt: nowISO(),
		})
		return
	}
}

func (p *Pipeline) runWatcher() {
	defer p.wg.Done()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	// Run one cycle immediately so we don't lose 2s at startup.
	p.scan(true)
	for {
		select {
		case <-p.stopCh:
			return
		case <-tick.C:
			p.scan(false)
		}
	}
}

// scan lists tasks from the embedded API; on the first ever cycle
// (empty state) it ingests every success without queuing them — the
// product decision is "only auto-transcribe NEW downloads, not old
// ones". Subsequent cycles enqueue anything freshly successful.
//
// scan also acts as a self-healer: any already-known Job whose Title
// or VideoPath drifted from sphkit's source of truth gets refreshed
// in-place. This recovers records that were persisted while an earlier
// version of the JSON parser produced empty values, without forcing
// the user to manually click Retry just to fix the displayed title.
// The repair only rewrites metadata — it does NOT auto-enqueue a
// re-transcribe, because the original file may legitimately be gone.
func (p *Pipeline) scan(initial bool) {
	kit := p.kitFn()
	if kit == nil {
		return
	}
	page, err := kit.ListTasks("all", 1, 200)
	if err != nil {
		return
	}
	firstEver := initial && !p.state.HasEverScanned()
	for _, t := range page.Tasks {
		p.repairJobMetadata(t)

		if !isSuccess(t.Status) {
			continue
		}
		if p.state.Seen(t.ID) {
			continue
		}
		if firstEver {
			// Ingest existing downloads as "seen, skipped" so we don't
			// retroactively transcribe the user's history.
			p.state.MarkSeen(t.ID)
			continue
		}
		// New success -> queue if auto is on; either way remember.
		p.state.MarkSeen(t.ID)
		if !p.AutoEnabled() {
			continue
		}
		p.upsertJob(Job{
			TaskID:    t.ID,
			Title:     t.Title,
			VideoPath: filepath.Join(t.Path, t.Filename),
			Source:    SourceWxChannel,
			Stage:     StagePending,
			CreatedAt: nowISO(),
			UpdatedAt: nowISO(),
		})
		select {
		case p.jobs <- t.ID:
		default:
		}
	}
	if firstEver {
		p.state.MarkScanned()
	}
	_ = p.state.Save()
}

// repairJobMetadata syncs the persisted Job's display fields with
// sphkit's current task data, if a Job exists for this taskID. We only
// touch Title and VideoPath — the Stage/Error/Progress are left alone
// so a previously-failed job stays failed (with the corrected metadata
// surfaced in the UI) until the user clicks Retry.
//
// External (yt-dlp) jobs are off-limits here: their metadata comes
// from the external manager, not sphkit.
func (p *Pipeline) repairJobMetadata(t sphkit.TaskSummary) {
	job, ok := p.state.Get(t.ID)
	if !ok {
		return
	}
	if job.Source == SourceExternal {
		return
	}
	newPath := filepath.Join(t.Path, t.Filename)
	changed := false
	if t.Title != "" && job.Title != t.Title {
		job.Title = t.Title
		changed = true
	}
	if newPath != "" && newPath != string(filepath.Separator) && job.VideoPath != newPath {
		job.VideoPath = newPath
		changed = true
	}
	if !changed {
		return
	}
	job.UpdatedAt = nowISO()
	p.upsertJob(job)
}

func (p *Pipeline) runWorker() {
	defer p.wg.Done()
	for {
		select {
		case <-p.stopCh:
			return
		case taskID := <-p.jobs:
			p.processOne(taskID)
		}
	}
}

func (p *Pipeline) processOne(taskID string) {
	p.mu.Lock()
	if _, ok := p.inFlight[taskID]; ok {
		p.mu.Unlock()
		return
	}
	p.inFlight[taskID] = struct{}{}
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.inFlight, taskID)
		p.mu.Unlock()
	}()

	job, ok := p.state.Get(taskID)
	if !ok {
		return
	}
	logbus.Info("pipeline", "transcribe start: %s (%s)", abbrevTitle(job.Title, job.TaskID), job.Source)
	started := time.Now()
	if err := p.runJob(&job); err != nil {
		job.Stage = StageFailed
		job.Error = err.Error()
		job.UpdatedAt = nowISO()
		p.upsertJob(job)
		logbus.Error("pipeline", "transcribe failed: %s — %v", abbrevTitle(job.Title, job.TaskID), err)
		return
	}
	job.Stage = StageDone
	job.Progress = 1
	job.Error = ""
	job.UpdatedAt = nowISO()
	p.upsertJob(job)
	logbus.Info("pipeline", "transcribe done in %s: %s", time.Since(started).Round(time.Millisecond), abbrevTitle(job.Title, job.TaskID))
}

// abbrevTitle returns a short, log-friendly identifier — the first
// 48 chars of the title, falling back to the taskID when title is
// empty. Keeps log lines from blowing up with full Chinese titles.
func abbrevTitle(title, taskID string) string {
	t := strings.TrimSpace(title)
	if t == "" {
		return taskID
	}
	runes := []rune(t)
	if len(runes) > 48 {
		t = string(runes[:48]) + "…"
	}
	return t
}

func (p *Pipeline) runJob(job *Job) error {
	if _, err := os.Stat(job.VideoPath); err != nil {
		return fmt.Errorf("video not found: %w", err)
	}

	// Stage 1: extract audio
	job.Stage = StageExtracting
	job.Progress = -1
	job.ProgressMsg = "提取音频"
	job.UpdatedAt = nowISO()
	p.upsertJob(*job)

	tmp, err := screbuntime.TempDir()
	if err != nil {
		return err
	}
	wav, err := media.ExtractAudio(p.ctx, job.VideoPath, tmp)
	if err != nil {
		return fmt.Errorf("extract: %w", err)
	}
	defer os.Remove(wav)

	// Stage 2: transcribe
	job.Stage = StageTranscribing
	job.ProgressMsg = "转写中"
	job.UpdatedAt = nowISO()
	p.upsertJob(*job)

	result, err := p.provider.Transcribe(p.ctx, transcribe.Request{
		AudioPath: wav,
		Language:  "auto",
		Model:     pickInstalledModel(transcribe.LoadPreferences().ActiveModel),
		OnProgress: func(frac float64, msg string) {
			cur, ok := p.state.Get(job.TaskID)
			if !ok {
				return
			}
			cur.Progress = frac
			cur.ProgressMsg = msg
			cur.UpdatedAt = nowISO()
			p.upsertJob(cur)
		},
	})
	if err != nil {
		return fmt.Errorf("transcribe: %w", err)
	}
	job.Model = result.Model
	job.Language = result.Language
	job.Duration = result.Duration

	// Stage 3: save (JSON + SRT). Apply glossary first so the saved
	// artifacts are already post-deterministic-replacement.
	job.Stage = StageSaving
	job.ProgressMsg = "保存"
	job.UpdatedAt = nowISO()
	p.upsertJob(*job)

	saved := applyGlossary(p.glossary, result)
	jsonPath, err := saveTranscriptJSON(job.TaskID, saved)
	if err != nil {
		return fmt.Errorf("save json: %w", err)
	}
	srtPath, err := saveSRT(job.VideoPath, result)
	if err != nil {
		// Non-fatal: JSON is our source of truth; SRT is a convenience.
		srtPath = ""
	}
	job.TranscriptPath = jsonPath
	job.SRTPath = srtPath
	return nil
}

func (p *Pipeline) upsertJob(j Job) {
	p.state.Upsert(j)
	_ = p.state.Save()
	runtime.EventsEmit(p.ctx, "transcribe:job", j)
}

func isSuccess(status string) bool {
	switch strings.ToLower(status) {
	case "success", "done", "completed":
		return true
	}
	return false
}

func nowISO() string { return time.Now().Format(time.RFC3339) }
