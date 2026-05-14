// SPDX-License-Identifier: GPL-3.0-or-later
// Package logbus is the centralised log sink for the Scribe app.
// It holds a bounded ring buffer of structured entries, broadcasts
// each new entry to the Wails frontend via "log:entry", and acts as
// an `io.Writer` so stdlib `log.SetOutput(bus)` redirects ad-hoc
// `log.Printf` calls into the same stream.
//
// Usage from inside the app:
//
//	logbus.Info("pipeline", "transcribe done in %s", elapsed)
//	logbus.Errorf("external", "yt-dlp: %v", err)
//
// The frontend subscribes to the "log:entry" event to live-tail.
package logbus

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Level mirrors the small set of severities the Logs UI renders.
// Kept as a string so JSON state is human-readable.
type Level string

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

// Entry is the unit the buffer holds and the event carries.
type Entry struct {
	Timestamp string `json:"timestamp"` // RFC3339Nano
	Level     Level  `json:"level"`
	Source    string `json:"source"`  // "pipeline" / "external" / "proxy" / "stdlib"
	Message   string `json:"message"` // free-form, no trailing newline
}

// Bus owns the ring buffer and the broadcast plumbing. There's one
// process-global Bus accessed via the package-level helpers
// (Info/Warn/Error/...); callers don't construct their own.
type Bus struct {
	mu      sync.Mutex
	entries []Entry
	cap     int
	ctx     context.Context
}

const defaultCapacity = 2000

// defaultBus is the singleton wired up by Init. Until Init is called,
// log helpers buffer entries in memory with no emit — they're safe to
// invoke from package init() blocks or early in Startup.
var defaultBus = &Bus{cap: defaultCapacity, entries: make([]Entry, 0, 256)}

// Init wires the Wails context so future entries broadcast to the UI.
// Safe to call exactly once during App.Startup. Replays nothing — the
// frontend pulls history via ListLogs() and then subscribes to the
// stream.
func Init(ctx context.Context) {
	defaultBus.mu.Lock()
	defaultBus.ctx = ctx
	defaultBus.mu.Unlock()
}

// SetCapacity changes the ring buffer cap. Useful for tests; the
// default (2000) is generous enough that an interactive session
// barely scrolls anything off the end.
func SetCapacity(n int) {
	if n <= 0 {
		return
	}
	defaultBus.mu.Lock()
	defaultBus.cap = n
	if len(defaultBus.entries) > n {
		defaultBus.entries = append([]Entry(nil), defaultBus.entries[len(defaultBus.entries)-n:]...)
	}
	defaultBus.mu.Unlock()
}

// List returns up to `limit` most-recent entries. If limit <= 0 or
// larger than the buffered count, the entire buffer is returned. The
// returned slice is a copy — callers can mutate it freely.
func List(limit int) []Entry {
	defaultBus.mu.Lock()
	defer defaultBus.mu.Unlock()
	n := len(defaultBus.entries)
	if limit <= 0 || limit > n {
		limit = n
	}
	start := n - limit
	out := make([]Entry, limit)
	copy(out, defaultBus.entries[start:])
	return out
}

// Clear empties the buffer. Used by the "清空" button in the UI.
// We emit a synthetic info entry afterwards so the frontend doesn't
// stay frozen on stale content if the live stream is quiet.
func Clear() {
	defaultBus.mu.Lock()
	defaultBus.entries = defaultBus.entries[:0]
	defaultBus.mu.Unlock()
	Info("logbus", "cleared")
}

// Debug/Info/Warn/Error/Errorf are the typical entry points. All
// share the same Printf-style varargs API so callers can write
//
//	logbus.Errorf("external", "yt-dlp exited: %v", err)
//
// without an extra fmt.Sprintf round trip at every call site.
func Debug(source, format string, args ...any) { write(LevelDebug, source, format, args...) }
func Info(source, format string, args ...any)  { write(LevelInfo, source, format, args...) }
func Warn(source, format string, args ...any)  { write(LevelWarn, source, format, args...) }
func Error(source, format string, args ...any) { write(LevelError, source, format, args...) }

// Errorf is an alias kept for callers that already imported it under
// that name (mirrors the stdlib `fmt.Errorf` convention).
func Errorf(source, format string, args ...any) { write(LevelError, source, format, args...) }

func write(level Level, source, format string, args ...any) {
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	msg = strings.TrimRight(msg, "\r\n")
	if msg == "" {
		return
	}
	e := Entry{
		Timestamp: time.Now().Format(time.RFC3339Nano),
		Level:     level,
		Source:    source,
		Message:   msg,
	}
	defaultBus.mu.Lock()
	defaultBus.entries = append(defaultBus.entries, e)
	if len(defaultBus.entries) > defaultBus.cap {
		// Trim from the front. Use a fresh slice rather than reslice
		// to drop the underlying array memory; the buffer is small
		// (default 2000 entries) so the copy cost is negligible.
		defaultBus.entries = append(
			make([]Entry, 0, defaultBus.cap),
			defaultBus.entries[len(defaultBus.entries)-defaultBus.cap:]...,
		)
	}
	ctx := defaultBus.ctx
	defaultBus.mu.Unlock()

	if ctx != nil {
		wailsruntime.EventsEmit(ctx, "log:entry", e)
	}
}

// stdlibWriter adapts the Bus into an io.Writer so callers can pipe
// the standard library `log` package (and any third-party code using
// it) into the same stream. Stdlib log lines look like:
//
//	2026/05/14 19:23:47 scribe: pipeline init: ...
//
// We split off the timestamp prefix the stdlib already added so the
// UI doesn't show it twice, and classify the entry by scanning for
// well-known prefixes (e.g. "scribe: ai settings load") to pick a
// source.
type stdlibWriter struct{}

// StdlibWriter returns the io.Writer the caller should hand to
// `log.SetOutput`. Lines may be batched by the caller — we split on
// newlines and emit one entry per non-empty line.
func StdlibWriter() *stdlibWriter { return &stdlibWriter{} }

func (w *stdlibWriter) Write(p []byte) (int, error) {
	text := string(p)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Strip the leading "YYYY/MM/DD HH:MM:SS " stdlib prefix.
		line = stripStdlibPrefix(line)
		// Pick a source heuristically: stdlib log calls in this
		// codebase use "scribe:" / "wx_channel:" / etc. as a
		// pseudo-tag. If we can't find one, fall back to "stdlib".
		source, msg := splitSourceTag(line)
		write(LevelInfo, source, "%s", msg)
	}
	return len(p), nil
}

// stripStdlibPrefix peels off the "2026/05/14 19:23:47 " prefix the
// stdlib log writer prepends. We're permissive — if the format
// changes (caller used log.SetFlags) we just return the line as-is.
func stripStdlibPrefix(line string) string {
	const minLen = len("2006/01/02 15:04:05 ")
	if len(line) < minLen {
		return line
	}
	// Cheap check: digits + slashes + colons at the right positions.
	pat := []int{4, 7, 13, 16, 19} // expected separator indices
	seps := []byte{'/', '/', ':', ':', ' '}
	for i, idx := range pat {
		if line[idx] != seps[i] {
			return line
		}
	}
	return strings.TrimSpace(line[minLen:])
}

func splitSourceTag(line string) (source, msg string) {
	if idx := strings.Index(line, ": "); idx > 0 && idx < 32 {
		tag := line[:idx]
		// Only treat single-word tags as source labels — avoids
		// catching real "Error: something" messages.
		if !strings.ContainsAny(tag, " \t") {
			return tag, line[idx+2:]
		}
	}
	return "stdlib", line
}
